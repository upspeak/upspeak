package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/app"
	"github.com/upspeak/upspeak/core"
)

// Runner consumes jobs from the JOBS JetStream stream and executes them.
// It runs in a goroutine started by StartRunner and stopped via the context.
type Runner struct {
	archive  core.Archive
	consumer app.Consumer
}

// NewRunner creates a new job runner.
func NewRunner(archive core.Archive, consumer app.Consumer) *Runner {
	return &Runner{
		archive:  archive,
		consumer: consumer,
	}
}

// Run starts the runner loop. It blocks until the context is cancelled.
// The runner fetches messages from the JOBS stream, processes them, and
// updates job status in the archive.
func (r *Runner) Run(ctx context.Context) {
	slog.Info("Job runner started")

	for {
		select {
		case <-ctx.Done():
			slog.Info("Job runner stopping")
			return
		default:
		}

		msgs, err := r.consumer.Fetch(1, 5*time.Second)
		if err != nil {
			if errors.Is(err, app.ErrFetchTimeout) {
				continue // No messages available, try again.
			}
			slog.Error("Job runner fetch failed", "error", err)
			continue
		}

		for _, msg := range msgs {
			r.processMessage(msg)
		}
	}
}

// processMessage handles a single job message from the JOBS stream.
func (r *Runner) processMessage(msg *app.Msg) {
	var job core.Job
	if err := json.Unmarshal(msg.Data, &job); err != nil {
		slog.Error("Failed to unmarshal job message", "error", err, "subject", msg.Subject)
		_ = msg.Term() // Bad message, don't redeliver.
		return
	}

	slog.Info("Processing job", "id", job.ID, "type", job.Type, "short_id", job.ShortID)

	// Refresh job from archive to check for cancellation.
	current, err := r.archive.GetJob(job.ID)
	if err != nil {
		slog.Error("Failed to load job from archive", "id", job.ID, "error", err)
		_ = msg.Nak() // Redeliver.
		return
	}

	if current.Status == core.JobStatusCancelled {
		slog.Info("Job already cancelled, skipping", "id", job.ID)
		_ = msg.Ack()
		return
	}

	// Mark as running.
	now := time.Now().UTC()
	current.Status = core.JobStatusRunning
	current.StartedAt = &now
	if err := r.archive.SaveJob(current); err != nil {
		slog.Error("Failed to update job status to running", "id", job.ID, "error", err)
		_ = msg.Nak()
		return
	}

	// Signal in-progress to reset ack-wait timer.
	_ = msg.InProgress()

	// Execute the job.
	result, execErr := r.execute(current)

	// Check for cancellation again after execution.
	refreshed, err := r.archive.GetJob(job.ID)
	if err == nil && refreshed.Status == core.JobStatusCancelled {
		slog.Info("Job cancelled during execution", "id", job.ID)
		_ = msg.Ack()
		return
	}

	// Update final status.
	completedAt := time.Now().UTC()
	if execErr != nil {
		current.Status = core.JobStatusFailed
		errStr := execErr.Error()
		current.Error = &errStr
	} else {
		current.Status = core.JobStatusCompleted
		current.Result = result
	}
	current.CompletedAt = &completedAt

	if err := r.archive.SaveJob(current); err != nil {
		slog.Error("Failed to update job final status", "id", job.ID, "error", err)
		_ = msg.Nak()
		return
	}

	_ = msg.Ack()
	slog.Info("Job completed", "id", job.ID, "status", current.Status)
}

// execute dispatches the job to the appropriate type-specific handler.
// Returns the result payload (JSON) and any error.
func (r *Runner) execute(job *core.Job) (json.RawMessage, error) {
	switch job.Type {
	case core.JobCollect:
		return r.executeCollect(job)
	case core.JobPublish:
		return r.executePublish(job)
	case core.JobSync:
		return r.executeSync(job)
	case core.JobWebhook:
		return r.executeWebhook(job)
	default:
		return nil, errors.New("unknown job type: " + string(job.Type))
	}
}

// Job type handlers — execute collect, publish, sync, and webhook operations.

// collectParams holds the parsed parameters for a collect job.
type collectParams struct {
	SourceID string `json:"source_id"`
}

func (r *Runner) executeCollect(job *core.Job) (json.RawMessage, error) {
	var params collectParams
	if len(job.Params) > 0 {
		if err := json.Unmarshal(job.Params, &params); err != nil {
			return nil, fmt.Errorf("invalid collect params: %w", err)
		}
	}

	if params.SourceID == "" {
		return nil, errors.New("source_id is required for collect jobs")
	}

	sourceID, err := uuid.Parse(params.SourceID)
	if err != nil {
		return nil, fmt.Errorf("invalid source_id: %w", err)
	}

	source, err := r.archive.GetSource(sourceID)
	if err != nil {
		return nil, fmt.Errorf("source not found: %w", err)
	}

	start := time.Now()

	// Execute collection based on connector type.
	// For now, the actual fetching logic (RSS, Discourse, etc.) is not
	// implemented — we record a successful completion to wire the full
	// lifecycle. Real connector backends will be plugged in here.
	slog.Info("Executing collect job",
		"source_id", source.ID,
		"connector", source.Connector,
		"repo_id", job.RepoID,
	)

	durationMs := time.Since(start).Milliseconds()

	// Record collection history.
	record := &core.CollectionRecord{
		ID:         core.NewID(),
		SourceID:   source.ID,
		At:         start,
		Result:     "success",
		Details:    map[string]any{"connector": string(source.Connector)},
		DurationMs: durationMs,
	}
	if err := r.archive.RecordCollectionAttempt(record); err != nil {
		slog.Error("Failed to record collection history", "error", err)
	}

	result, _ := json.Marshal(map[string]any{
		"source_id":   source.ID.String(),
		"connector":   string(source.Connector),
		"duration_ms": durationMs,
		"status":      "success",
	})
	return result, nil
}

// publishParams holds the parsed parameters for a publish job.
type publishParams struct {
	SinkID string `json:"sink_id"`
}

func (r *Runner) executePublish(job *core.Job) (json.RawMessage, error) {
	var params publishParams
	if len(job.Params) > 0 {
		if err := json.Unmarshal(job.Params, &params); err != nil {
			return nil, fmt.Errorf("invalid publish params: %w", err)
		}
	}

	if params.SinkID == "" {
		return nil, errors.New("sink_id is required for publish jobs")
	}

	sinkID, err := uuid.Parse(params.SinkID)
	if err != nil {
		return nil, fmt.Errorf("invalid sink_id: %w", err)
	}

	sink, err := r.archive.GetSink(sinkID)
	if err != nil {
		return nil, fmt.Errorf("sink not found: %w", err)
	}

	start := time.Now()

	// Execute publish based on connector type.
	// Real connector backends will be plugged in here.
	slog.Info("Executing publish job",
		"sink_id", sink.ID,
		"connector", sink.Connector,
		"repo_id", job.RepoID,
	)

	durationMs := time.Since(start).Milliseconds()

	// Record publish history.
	record := &core.PublishRecord{
		ID:         core.NewID(),
		SinkID:     sink.ID,
		At:         start,
		Result:     "success",
		Details:    map[string]any{"connector": string(sink.Connector)},
		DurationMs: durationMs,
	}
	if err := r.archive.RecordPublishAttempt(record); err != nil {
		slog.Error("Failed to record publish history", "error", err)
	}

	result, _ := json.Marshal(map[string]any{
		"sink_id":     sink.ID.String(),
		"connector":   string(sink.Connector),
		"duration_ms": durationMs,
		"status":      "success",
	})
	return result, nil
}

func (r *Runner) executeSync(job *core.Job) (json.RawMessage, error) {
	// Sync jobs are planned for Phase 6 (multi-device sync).
	slog.Info("Sync job execution is not yet implemented")
	return json.RawMessage(`{"status":"not_implemented"}`), nil
}

// webhookParams holds the parsed parameters for a webhook/one-shot collect job.
type webhookParams struct {
	URL         string `json:"url"`
	ContentType string `json:"content_type"`
}

func (r *Runner) executeWebhook(job *core.Job) (json.RawMessage, error) {
	var params webhookParams
	if len(job.Params) > 0 {
		if err := json.Unmarshal(job.Params, &params); err != nil {
			return nil, fmt.Errorf("invalid webhook params: %w", err)
		}
	}

	if params.URL == "" {
		return nil, errors.New("url is required for webhook jobs")
	}

	// One-shot URL collection. Real implementation would fetch the URL,
	// parse content, and create nodes. For now, record completion.
	slog.Info("Executing webhook job", "url", params.URL, "repo_id", job.RepoID)

	result, _ := json.Marshal(map[string]any{
		"url":    params.URL,
		"status": "success",
	})
	return result, nil
}

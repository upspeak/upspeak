package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/upspeak/upspeak/app"
	"github.com/upspeak/upspeak/core"
)

// Runner consumes jobs from the JOBS JetStream stream and executes them.
// It runs in a goroutine started by StartRunner and stopped via the context.
type Runner struct {
	archive  core.Archive
	consumer app.Consumer
	logger   *slog.Logger
}

// NewRunner creates a new job runner.
func NewRunner(archive core.Archive, consumer app.Consumer, logger *slog.Logger) *Runner {
	return &Runner{
		archive:  archive,
		consumer: consumer,
		logger:   logger,
	}
}

// Run starts the runner loop. It blocks until the context is cancelled.
// The runner fetches messages from the JOBS stream, processes them, and
// updates job status in the archive.
func (r *Runner) Run(ctx context.Context) {
	r.logger.Info("Job runner started")

	for {
		select {
		case <-ctx.Done():
			r.logger.Info("Job runner stopping")
			return
		default:
		}

		msgs, err := r.consumer.Fetch(1, 5*time.Second)
		if err != nil {
			if errors.Is(err, app.ErrFetchTimeout) {
				continue // No messages available, try again.
			}
			r.logger.Error("Job runner fetch failed", "error", err)
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
		r.logger.Error("Failed to unmarshal job message", "error", err, "subject", msg.Subject)
		_ = msg.Term() // Bad message, don't redeliver.
		return
	}

	r.logger.Info("Processing job", "id", job.ID, "type", job.Type, "short_id", job.ShortID)

	// Refresh job from archive to check for cancellation.
	current, err := r.archive.GetJob(job.ID)
	if err != nil {
		r.logger.Error("Failed to load job from archive", "id", job.ID, "error", err)
		_ = msg.Nak() // Redeliver.
		return
	}

	if current.Status == core.JobStatusCancelled {
		r.logger.Info("Job already cancelled, skipping", "id", job.ID)
		_ = msg.Ack()
		return
	}

	// Mark as running.
	now := time.Now().UTC()
	current.Status = core.JobStatusRunning
	current.StartedAt = &now
	if err := r.archive.SaveJob(current); err != nil {
		r.logger.Error("Failed to update job status to running", "id", job.ID, "error", err)
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
		r.logger.Info("Job cancelled during execution", "id", job.ID)
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
		r.logger.Error("Failed to update job final status", "id", job.ID, "error", err)
		_ = msg.Nak()
		return
	}

	_ = msg.Ack()
	r.logger.Info("Job completed", "id", job.ID, "status", current.Status)
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

// Job type handlers — stubbed until Phase 4 (connectors) implements them.

func (r *Runner) executeCollect(_ *core.Job) (json.RawMessage, error) {
	r.logger.Info("Collect job execution is not yet implemented")
	return json.RawMessage(`{"status":"not_implemented"}`), nil
}

func (r *Runner) executePublish(_ *core.Job) (json.RawMessage, error) {
	r.logger.Info("Publish job execution is not yet implemented")
	return json.RawMessage(`{"status":"not_implemented"}`), nil
}

func (r *Runner) executeSync(_ *core.Job) (json.RawMessage, error) {
	r.logger.Info("Sync job execution is not yet implemented")
	return json.RawMessage(`{"status":"not_implemented"}`), nil
}

func (r *Runner) executeWebhook(_ *core.Job) (json.RawMessage, error) {
	r.logger.Info("Webhook job execution is not yet implemented")
	return json.RawMessage(`{"status":"not_implemented"}`), nil
}

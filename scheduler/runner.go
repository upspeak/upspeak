package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/app"
	"github.com/upspeak/upspeak/core"
	"github.com/upspeak/upspeak/jobs"
)

// triggerMessage is the payload published to the SCHEDULES stream when a
// schedule fires. The consumer reads this to create the corresponding job.
type triggerMessage struct {
	ScheduleID uuid.UUID `json:"schedule_id"`
	FiredAt    time.Time `json:"fired_at"`
}

// Runner handles timed execution of schedules. It runs two loops:
//   - tickLoop: checks enabled schedules every minute and publishes trigger messages
//   - consumeLoop: fetches trigger messages from SCHEDULES stream and creates jobs
type Runner struct {
	archive  core.Archive
	pub      app.Publisher
	consumer app.Consumer
	logger   *slog.Logger
}

// NewRunner creates a new schedule runner.
func NewRunner(archive core.Archive, pub app.Publisher, consumer app.Consumer, logger *slog.Logger) *Runner {
	return &Runner{
		archive:  archive,
		pub:      pub,
		consumer: consumer,
		logger:   logger,
	}
}

// Run starts the runner. It blocks until the context is cancelled.
// The tick loop runs in a separate goroutine; the consume loop runs
// in the calling goroutine.
func (r *Runner) Run(ctx context.Context) {
	r.logger.Info("Schedule runner started")

	go r.tickLoop(ctx)
	r.consumeLoop(ctx)
}

// tickLoop checks enabled schedules every minute and publishes trigger messages
// for any schedule whose NextRun has elapsed.
func (r *Runner) tickLoop(ctx context.Context) {
	// Perform an initial check immediately.
	r.checkSchedules()

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.logger.Info("Schedule tick loop stopping")
			return
		case <-ticker.C:
			r.checkSchedules()
		}
	}
}

// checkSchedules queries all enabled schedules and publishes trigger messages
// for those whose NextRun time has passed.
func (r *Runner) checkSchedules() {
	schedules, err := r.archive.GetEnabledSchedules()
	if err != nil {
		r.logger.Error("Failed to get enabled schedules", "error", err)
		return
	}

	now := time.Now().UTC()
	for i := range schedules {
		s := &schedules[i]
		if s.NextRun == nil || s.NextRun.After(now) {
			continue
		}

		r.publishTrigger(s, now)
	}
}

// publishTrigger publishes a trigger message for the given schedule and updates
// its LastRun and NextRun fields in the archive.
func (r *Runner) publishTrigger(schedule *core.Schedule, now time.Time) {
	msg := triggerMessage{
		ScheduleID: schedule.ID,
		FiredAt:    now,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		r.logger.Error("Failed to marshal trigger message", "schedule_id", schedule.ID, "error", err)
		return
	}

	subject := "schedules.trigger." + schedule.ID.String()
	if err := r.pub.Publish(subject, data); err != nil {
		r.logger.Error("Failed to publish schedule trigger", "schedule_id", schedule.ID, "error", err)
		return
	}

	// Update LastRun and compute new NextRun.
	schedule.LastRun = &now
	cs, err := ParseCron(schedule.Cron)
	if err != nil {
		r.logger.Error("Failed to parse cron for next run", "schedule_id", schedule.ID, "error", err)
		schedule.NextRun = nil
	} else {
		next := cs.Next(now)
		if next.IsZero() {
			schedule.NextRun = nil
		} else {
			schedule.NextRun = &next
		}
	}

	if err := r.archive.SaveSchedule(schedule); err != nil {
		r.logger.Error("Failed to update schedule after trigger", "schedule_id", schedule.ID, "error", err)
	}
}

// consumeLoop fetches trigger messages from the SCHEDULES stream and creates
// jobs for each triggered schedule. It blocks until the context is cancelled.
func (r *Runner) consumeLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			r.logger.Info("Schedule consume loop stopping")
			return
		default:
		}

		msgs, err := r.consumer.Fetch(5, 5*time.Second)
		if err != nil {
			if errors.Is(err, app.ErrFetchTimeout) {
				continue
			}
			r.logger.Error("Schedule runner fetch failed", "error", err)
			continue
		}

		for _, msg := range msgs {
			r.processTrigger(msg)
		}
	}
}

// processTrigger handles a single trigger message from the SCHEDULES stream.
// It loads the schedule, determines the job type, and creates a job.
func (r *Runner) processTrigger(msg *app.Msg) {
	var trigger triggerMessage
	if err := json.Unmarshal(msg.Data, &trigger); err != nil {
		r.logger.Error("Failed to unmarshal trigger message", "error", err, "subject", msg.Subject)
		_ = msg.Term()
		return
	}

	schedule, err := r.archive.GetSchedule(trigger.ScheduleID)
	if err != nil {
		r.logger.Error("Failed to load schedule for trigger", "schedule_id", trigger.ScheduleID, "error", err)
		_ = msg.Term() // Schedule may have been deleted; don't redeliver.
		return
	}

	// Skip if schedule has been disabled since the trigger was published.
	if !schedule.Enabled {
		r.logger.Info("Schedule disabled, skipping trigger", "schedule_id", schedule.ID)
		_ = msg.Ack()
		return
	}

	jobType := actionTypeToJobType(schedule.Action.Type)
	if jobType == "" {
		r.logger.Error("Unknown action type for schedule", "schedule_id", schedule.ID, "action_type", schedule.Action.Type)
		_ = msg.Term()
		return
	}

	repoID := uuid.Nil
	if schedule.Action.RepoID != nil {
		repoID = *schedule.Action.RepoID
	}

	job, err := jobs.CreateJob(r.archive, r.pub, repoID, schedule.CreatedBy, jobType, buildScheduleParams(schedule))
	if err != nil {
		r.logger.Error("Failed to create job from schedule trigger", "schedule_id", schedule.ID, "error", err)
		_ = msg.Nak() // Redeliver to retry.
		return
	}

	r.logger.Info("Created job from schedule trigger",
		"schedule_id", schedule.ID,
		"job_id", job.ID,
		"job_type", job.Type,
	)

	_ = msg.Ack()
}

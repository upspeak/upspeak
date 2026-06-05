package core

import (
	"fmt"

	"github.com/google/uuid"
)

// Subject construction for the work-queue and trigger streams. The domain owns
// the wire scheme in one place — mirroring Event.Subject for repository events —
// so producers and transport never hand-build subject strings that could drift
// from the streams' configured subject filters.

// JobSubject returns the subject a new job is published on for the job runner to
// consume. Format: jobs.{job_type}.{job_id}, captured by the JOBS stream's
// jobs.> filter.
func JobSubject(jobType JobType, jobID uuid.UUID) string {
	return fmt.Sprintf("jobs.%s.%s", jobType, jobID.String())
}

// ScheduleTriggerSubject returns the subject a schedule trigger is published on
// for the schedule runner to consume. Format: schedules.trigger.{schedule_id},
// captured by the SCHEDULES stream's schedules.trigger.> filter.
func ScheduleTriggerSubject(scheduleID uuid.UUID) string {
	return fmt.Sprintf("schedules.trigger.%s", scheduleID.String())
}

// Package scheduler provides the scheduler module for managing cron-based
// triggers. Schedules are global entities that fire at specified intervals,
// creating jobs that execute actions (collect, publish, webhook).
package scheduler

import (
	"log/slog"

	"github.com/upspeak/upspeak/app"
	"github.com/upspeak/upspeak/core"
)

// Module implements the app.Module interface for schedule management.
// It exposes HTTP endpoints for creating, listing, updating, and triggering
// schedules. The Runner (see runner.go) handles timed execution via NATS.
type Module struct {
	archive  core.Archive
	pub      app.Publisher
	consumer app.Consumer
}

// Name returns the module name.
func (m *Module) Name() string {
	return "scheduler"
}

// Init initialises the scheduler module.
func (m *Module) Init(_ map[string]any) error {
	slog.Info("Initialised scheduler module")
	return nil
}

// SetArchive injects the archive dependency.
func (m *Module) SetArchive(archive core.Archive) {
	m.archive = archive
}

// SetPublisher injects the JetStream publisher for schedule events.
func (m *Module) SetPublisher(pub app.Publisher) {
	m.pub = pub
}

// SetConsumer injects the JetStream consumer for the SCHEDULES stream.
func (m *Module) SetConsumer(consumer app.Consumer) {
	m.consumer = consumer
}

// HTTPHandlers returns the HTTP handlers for the scheduler module.
// All paths are relative to the module's mount point (/api/v1).
func (m *Module) HTTPHandlers() []app.HTTPHandler {
	return []app.HTTPHandler{
		{Method: "POST", Path: "/schedules", Handler: m.createScheduleHandler()},
		{Method: "GET", Path: "/schedules", Handler: m.listSchedulesHandler()},
		{Method: "GET", Path: "/schedules/{schedule_ref}", Handler: m.getScheduleHandler()},
		{Method: "PUT", Path: "/schedules/{schedule_ref}", Handler: m.updateScheduleHandler()},
		{Method: "DELETE", Path: "/schedules/{schedule_ref}", Handler: m.deleteScheduleHandler()},
		{Method: "POST", Path: "/schedules/{schedule_ref}/trigger", Handler: m.triggerScheduleHandler()},
		{Method: "POST", Path: "/schedules/{schedule_ref}/pause", Handler: m.pauseScheduleHandler()},
		{Method: "POST", Path: "/schedules/{schedule_ref}/resume", Handler: m.resumeScheduleHandler()},
	}
}

// MsgHandlers returns the message handlers for the scheduler module.
func (m *Module) MsgHandlers() []app.MsgHandler {
	return nil
}

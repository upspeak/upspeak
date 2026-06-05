package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/upspeak/upspeak/app"
	"github.com/upspeak/upspeak/archive"
	"github.com/upspeak/upspeak/connector"
	"github.com/upspeak/upspeak/filter"
	"github.com/upspeak/upspeak/ingest"
	"github.com/upspeak/upspeak/integrations/webhook"
	"github.com/upspeak/upspeak/jobs"
	usnats "github.com/upspeak/upspeak/nats"
	"github.com/upspeak/upspeak/realtime"
	"github.com/upspeak/upspeak/repo"
	"github.com/upspeak/upspeak/rules"
	"github.com/upspeak/upspeak/scheduler"
	"github.com/upspeak/upspeak/search"
)

func main() {
	// Bootstrap the single application logger. Every component logs through this
	// default via package-level slog functions and never creates its own.
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	config, err := app.LoadConfig("upspeak.yaml")
	if err != nil {
		slog.Error("Error loading config", "error", err)
		os.Exit(1)
	}

	// Start NATS infrastructure.
	natsConfig := usnats.Config{
		URL:      config.NATS.URL,
		Embedded: config.NATS.Embedded,
		Private:  config.NATS.Private,
		Logging:  config.NATS.Logging,
	}
	bus, err := usnats.Start(config.Name, natsConfig)
	if err != nil {
		slog.Error("Error starting NATS", "error", err)
		os.Exit(1)
	}
	defer bus.Stop()

	// Create app.
	up := app.New(*config)
	up.SetSubscriber(bus.Subscriber())

	// Initialise archive module.
	archiveModule := &archive.ModuleArchive{}

	// Initialise repo module.
	repoModule := &repo.Module{}
	repoModule.SetPublisher(bus.Publisher())

	// Initialise filter module.
	filterModule := &filter.Module{}
	filterModule.SetPublisher(bus.Publisher())

	// Initialise jobs module.
	jobsModule := &jobs.Module{}

	// Initialise connector module.
	connectorModule := &connector.Module{}
	connectorModule.SetPublisher(bus.Publisher())

	// Initialise scheduler module.
	schedulerModule := &scheduler.Module{}
	schedulerModule.SetPublisher(bus.Publisher())

	// Initialise rules module.
	rulesModule := &rules.Module{}
	rulesModule.SetPublisher(bus.Publisher())

	// Initialise search module.
	searchModule := &search.Module{}

	// Initialise realtime module. Its events arrive via the app subscriber
	// (repo.*.events.> core-NATS fan-out), so it needs no dedicated consumer.
	realtimeModule := realtime.New()

	// Register modules.
	if err := up.AddModule(archiveModule); err != nil {
		slog.Error("Error adding archive module", "error", err)
		os.Exit(1)
	}

	if err := up.AddModuleOnPath(repoModule, "/api/v1"); err != nil {
		slog.Error("Error adding repo module", "error", err)
		os.Exit(1)
	}

	if err := up.AddModuleOnPath(filterModule, "/api/v1"); err != nil {
		slog.Error("Error adding filter module", "error", err)
		os.Exit(1)
	}

	if err := up.AddModuleOnPath(jobsModule, "/api/v1"); err != nil {
		slog.Error("Error adding jobs module", "error", err)
		os.Exit(1)
	}

	if err := up.AddModuleOnPath(connectorModule, "/api/v1"); err != nil {
		slog.Error("Error adding connector module", "error", err)
		os.Exit(1)
	}

	if err := up.AddModuleOnPath(schedulerModule, "/api/v1"); err != nil {
		slog.Error("Error adding scheduler module", "error", err)
		os.Exit(1)
	}

	if err := up.AddModuleOnPath(rulesModule, "/api/v1"); err != nil {
		slog.Error("Error adding rules module", "error", err)
		os.Exit(1)
	}

	if err := up.AddModuleOnPath(searchModule, "/api/v1"); err != nil {
		slog.Error("Error adding search module", "error", err)
		os.Exit(1)
	}

	if err := up.AddModuleOnPath(realtimeModule, "/api/v1"); err != nil {
		slog.Error("Error adding realtime module", "error", err)
		os.Exit(1)
	}

	// Initialise modules (calls Init, registers handlers, but does NOT start HTTP).
	if err := up.InitModules(); err != nil {
		slog.Error("Error initialising modules", "error", err)
		os.Exit(1)
	}

	// Wire cross-module dependencies after Init but before HTTP starts.
	a := archiveModule.GetArchive()
	repoModule.SetArchive(a)
	filterModule.SetArchive(a)
	jobsModule.SetArchive(a)
	connectorModule.SetArchive(a)
	schedulerModule.SetArchive(a)
	rulesModule.SetArchive(a)
	searchModule.SetArchive(a)
	realtimeModule.SetArchive(a)

	// Set up NATS JetStream streams and consumers for job processing.
	sm := usnats.NewStreamManager(bus)
	if err := sm.CreateJobsStream(); err != nil {
		slog.Error("Error creating JOBS stream", "error", err)
		os.Exit(1)
	}
	if err := sm.CreateSchedulesStream(); err != nil {
		slog.Error("Error creating SCHEDULES stream", "error", err)
		os.Exit(1)
	}
	if err := sm.CreateRepoEventsStream(); err != nil {
		slog.Error("Error creating REPO_EVENTS stream", "error", err)
		os.Exit(1)
	}
	cm := usnats.NewConsumerManager(bus)
	if err := cm.CreateJobRunnerConsumer(); err != nil {
		slog.Error("Error creating job-runner consumer", "error", err)
		os.Exit(1)
	}
	if err := cm.CreateScheduleRunnerConsumer(); err != nil {
		slog.Error("Error creating schedule-runner consumer", "error", err)
		os.Exit(1)
	}
	if err := cm.CreateRulesEngineConsumer(); err != nil {
		slog.Error("Error creating rules-engine consumer", "error", err)
		os.Exit(1)
	}

	// Create a JetStream consumer for the job runner.
	jobConsumer, err := usnats.NewConsumer(bus, "jobs.>", usnats.ConsumerJobRunner)
	if err != nil {
		slog.Error("Error creating job consumer", "error", err)
		os.Exit(1)
	}
	jobsModule.SetConsumer(jobConsumer)

	// Create a JetStream consumer for the schedule runner.
	scheduleConsumer, err := usnats.NewConsumer(bus, "schedules.trigger.>", usnats.ConsumerScheduleRunner)
	if err != nil {
		slog.Error("Error creating schedule consumer", "error", err)
		os.Exit(1)
	}
	schedulerModule.SetConsumer(scheduleConsumer)

	// The rules engine consumes the durable REPO_EVENTS stream. Created after
	// archive wiring so its loop never runs before dependencies are set; durable
	// delivery means events are not lost across restarts.
	rulesConsumer, err := usnats.NewConsumer(bus, usnats.RepoEventsSubject, usnats.ConsumerRulesEngine)
	if err != nil {
		slog.Error("Error creating rules-engine consumer subscription", "error", err)
		os.Exit(1)
	}

	// Build the adapter registry from the compiled-in integrations. main.go is
	// the only place that knows the concrete adapters; jobs/connector consume
	// the app.AdapterRegistry interface, so no import cycle forms.
	adapterRegistry := ingest.NewRegistry()
	adapterRegistry.Register(webhook.New())

	// Start every background loop uniformly through the app.Runner contract.
	// Events buffered before the realtime hub starts are drained once it runs;
	// all loops stop when runnerCtx is cancelled.
	runnerCtx, cancelRunner := context.WithCancel(context.Background())
	runners := []app.Runner{
		jobs.NewRunner(a, jobConsumer, bus.Publisher(), adapterRegistry),
		scheduler.NewRunner(a, bus.Publisher(), scheduleConsumer),
		rules.NewEngine(a, bus.Publisher(), rulesConsumer),
		realtimeModule,
	}
	for _, r := range runners {
		go r.Run(runnerCtx)
	}

	// Start HTTP server.
	if err := up.Start(); err != nil {
		slog.Error("Error starting app", "error", err)
		os.Exit(1)
	}

	// Wait for interrupt signal to gracefully shut down.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("Shutting down...")
	cancelRunner() // Stop job runner and schedule runner before draining NATS.
	if err := up.Stop(); err != nil {
		slog.Error("Error stopping app", "error", err)
		os.Exit(1)
	}
}

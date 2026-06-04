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
	"github.com/upspeak/upspeak/jobs"
	usnats "github.com/upspeak/upspeak/nats"
	"github.com/upspeak/upspeak/repo"
	"github.com/upspeak/upspeak/rules"
	"github.com/upspeak/upspeak/scheduler"
	"github.com/upspeak/upspeak/search"
)

func main() {
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

	// Start the job runner in a background goroutine.
	runnerCtx, cancelRunner := context.WithCancel(context.Background())
	runner := jobs.NewRunner(a, jobConsumer, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	go runner.Run(runnerCtx)

	// Start the schedule runner in a background goroutine.
	schedRunner := scheduler.NewRunner(a, bus.Publisher(), scheduleConsumer, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	go schedRunner.Run(runnerCtx)

	// Start the rules engine as a durable consumer on the REPO_EVENTS stream.
	// Created after archive wiring so its loop never runs before dependencies are
	// set; durable delivery means events are not lost across restarts.
	rulesConsumer, err := usnats.NewConsumer(bus, usnats.RepoEventsSubject, usnats.ConsumerRulesEngine)
	if err != nil {
		slog.Error("Error creating rules-engine consumer subscription", "error", err)
		os.Exit(1)
	}
	rulesEngine := rules.NewEngine(a, bus.Publisher(), rulesConsumer, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	go rulesEngine.Run(runnerCtx)

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

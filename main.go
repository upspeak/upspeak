package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/upspeak/upspeak/app"
	"github.com/upspeak/upspeak/archive"
	"github.com/upspeak/upspeak/filter"
	"github.com/upspeak/upspeak/jobs"
	usnats "github.com/upspeak/upspeak/nats"
	"github.com/upspeak/upspeak/repo"
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

	// Set up NATS JetStream streams and consumers for job processing.
	sm := usnats.NewStreamManager(bus)
	if err := sm.CreateJobsStream(); err != nil {
		slog.Error("Error creating JOBS stream", "error", err)
		os.Exit(1)
	}
	cm := usnats.NewConsumerManager(bus)
	if err := cm.CreateJobRunnerConsumer(); err != nil {
		slog.Error("Error creating job-runner consumer", "error", err)
		os.Exit(1)
	}

	// Create a JetStream consumer for the job runner.
	jobConsumer, err := usnats.NewConsumer(bus, "jobs.>", usnats.ConsumerJobRunner)
	if err != nil {
		slog.Error("Error creating job consumer", "error", err)
		os.Exit(1)
	}
	jobsModule.SetConsumer(jobConsumer)

	// Start the job runner in a background goroutine.
	runnerCtx, cancelRunner := context.WithCancel(context.Background())
	runner := jobs.NewRunner(a, jobConsumer, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	go runner.Run(runnerCtx)

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
	cancelRunner() // Stop job runner before draining NATS.
	if err := up.Stop(); err != nil {
		slog.Error("Error stopping app", "error", err)
		os.Exit(1)
	}
}

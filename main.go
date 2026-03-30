package main

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/upspeak/upspeak/app"
	"github.com/upspeak/upspeak/archive"
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

	// Register modules.
	if err := up.AddModule(archiveModule); err != nil {
		slog.Error("Error adding archive module", "error", err)
		os.Exit(1)
	}

	if err := up.AddModuleOnPath(repoModule, "/api/v1"); err != nil {
		slog.Error("Error adding repo module", "error", err)
		os.Exit(1)
	}

	// Start app (initialises modules, starts HTTP).
	if err := up.Start(); err != nil {
		slog.Error("Error starting app", "error", err)
		os.Exit(1)
	}

	// Wire dependencies after modules are initialised.
	repoModule.SetArchive(archiveModule.GetArchive())

	// Wait for interrupt signal to gracefully shut down.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("Shutting down...")
	if err := up.Stop(); err != nil {
		slog.Error("Error stopping app", "error", err)
		os.Exit(1)
	}
}

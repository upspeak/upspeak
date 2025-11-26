package main

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/upspeak/upspeak/app"
	"github.com/upspeak/upspeak/archive"
	"github.com/upspeak/upspeak/repo"
	"github.com/upspeak/upspeak/ui"
)

func main() {
	config, err := app.LoadConfig("upspeak.yaml")
	if err != nil {
		slog.Error("Error loading config", "error", err)
		os.Exit(1)
	}
	up := app.New(*config)

	// Initialise archive module
	archiveModule := &archive.ModuleArchive{}

	// Initialise repo module
	repoModule := &repo.ModuleRepo{}

	// Load modules
	// Add archive module first (no HTTP endpoints)
	if err := up.AddModule(archiveModule); err != nil {
		slog.Error("Error adding archive module", "error", err)
		os.Exit(1)
	}

	// Add repo module at /repo path
	if err := up.AddModuleOnPath(repoModule, "/repo"); err != nil {
		slog.Error("Error adding repo module", "error", err)
		os.Exit(1)
	}

	// Add UI module on root path
	if err := up.AddModuleOnPath(&ui.ModuleUI{}, "/"); err != nil {
		slog.Error("Error adding UI module on root", "error", err)
		os.Exit(1)
	}

	if err := up.Start(); err != nil {
		slog.Error("Error starting app", "error", err)
		os.Exit(1)
	}

	// Wire archive and repo together after initialisation
	// The handlers reference m.repo, so updating it here will make them use the new repository
	repoModule.SetArchive(archiveModule.GetArchive())

	// Wait for interrupt signal to gracefully shut down
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("Shutting down...")
	if err := up.Stop(); err != nil {
		slog.Error("Error stopping app", "error", err)
		os.Exit(1)
	}
}

package main

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/upspeak/upspeak/app"
	"github.com/upspeak/upspeak/ui"
)

func main() {
	config, err := app.LoadConfig("upspeak.yaml")
	if err != nil {
		slog.Error("Error loading config", "error", err)
		os.Exit(1)
	}
	up := app.New(*config)

	// Load modules
	if err := up.AddModuleOnPath(&ui.ModuleUI{}, "/"); err != nil {
		slog.Error("Error adding UI module on root", "error", err)
		os.Exit(1)
	}

	if err := up.Start(); err != nil {
		slog.Error("Error starting app", "error", err)
		os.Exit(1)
	}

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

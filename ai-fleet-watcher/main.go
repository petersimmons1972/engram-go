package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/petersimmons1972/ai-fleet-watcher/internal"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	hostname := os.Getenv("HOSTNAME")
	if hostname == "" {
		slog.Error("HOSTNAME env var required")
		os.Exit(1)
	}
	controllerURL := os.Getenv("CONTROLLER_URL")
	if controllerURL == "" {
		controllerURL = "https://ai-fleet.petersimmons.com"
	}

	docker, err := internal.NewDockerManager()
	if err != nil {
		slog.Error("docker manager", "err", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	slog.Info("ai-fleet-watcher starting", "hostname", hostname, "controller", controllerURL)
	internal.NewWatcher(hostname, controllerURL, docker).Run(ctx)
	slog.Info("ai-fleet-watcher stopped")
}

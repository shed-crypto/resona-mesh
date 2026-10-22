package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"resonamesh/internal/config"
)

const (
	AppVersion = "0.1.0-alpha"
	AppName    = "ResonaMesh"
)

func main() {
	for _, arg := range os.Args[1:] {
		if arg == "--version" || arg == "-v" {
			fmt.Printf("%s v%s\n", AppName, AppVersion)
			os.Exit(0)
		}
	}

	cfg, err := config.Load(".env", os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
		os.Exit(1)
	}

	logLevel := slog.LevelInfo
	if cfg.LogLevel == "DEBUG" {
		logLevel = slog.LevelDebug
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))
	slog.SetDefault(logger)

	slog.Info("Starting ResonaMesh Node",
		"app", AppName,
		"version", AppVersion,
		"http_addr", cfg.HTTPAddr,
		"p2p_port", cfg.P2PPort,
		"data_dir", cfg.DataDir,
		"quota_bytes", cfg.StorageQuotaBytes,
		"keeper_enabled", cfg.KeeperEnabled,
	)

	// Context with OS interrupt cancellation for graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Ensure data directory exists
	if err := os.MkdirAll(cfg.DataDir, 0700); err != nil {
		slog.Error("Failed to initialize data directory", "path", cfg.DataDir, "error", err)
		os.Exit(1)
	}

	slog.Info("ResonaMesh node initialized successfully. Awaiting shutdown signal...")

	// Block until interrupt signal received
	<-ctx.Done()

	slog.Info("Shutdown signal received. Commencing graceful teardown...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Teardown steps will cleanly flush SQLite WAL and disconnect libp2p streams
	_ = shutdownCtx

	slog.Info("ResonaMesh node shutdown completed cleanly.")
}

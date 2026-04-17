// Command manager is the entrypoint for the LX Container Weaver manager
// service. It loads configuration, initialises structured logging, starts the
// HTTP server and the orchestration loop, and handles graceful shutdown on
// SIGTERM/SIGINT.
//
// Directory conventions (ADR-009):
//
//	cmd/manager/      — binary entrypoint only; minimal logic here
//	internal/api/     — HTTP handler registration and server lifecycle
//	internal/config/  — environment-based configuration loading and validation
//	internal/orchestrator/ — per-cluster reconciliation loop (ADR-006)
//	internal/persistence/  — repository interfaces and domain models (ADR-004)
//	internal/provider/     — HyperscalerProvider interface and implementations (ADR-005)
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Mewtos7/lx-container-weaver/internal/api"
	"github.com/Mewtos7/lx-container-weaver/internal/config"
	"github.com/Mewtos7/lx-container-weaver/internal/orchestrator"
)

func main() {
	// -------------------------------------------------------------------------
	// Bootstrap: load configuration first so we know the log level.
	// -------------------------------------------------------------------------
	cfg, err := config.Load()
	if err != nil {
		// Use a plain text logger at this stage — structured logging hasn't
		// been initialised yet.
		slog.Error("configuration error", "error", err)
		os.Exit(1)
	}

	// -------------------------------------------------------------------------
	// Structured logging: initialise a JSON logger at the configured level and
	// replace the default logger so all packages using slog.Default() inherit it.
	// -------------------------------------------------------------------------
	logger := newLogger(cfg.LogLevel)
	slog.SetDefault(logger)

	logger.Info("manager starting",
		"http_addr", cfg.HTTPAddr,
		"log_level", cfg.LogLevel,
		"reconcile_interval", cfg.ReconcileInterval,
	)

	// -------------------------------------------------------------------------
	// Signal handling: create a root context that is cancelled on SIGTERM or
	// SIGINT so that all child goroutines receive a clean shutdown signal.
	// -------------------------------------------------------------------------
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	// -------------------------------------------------------------------------
	// Orchestration loop: run in a separate goroutine; terminates when ctx is
	// cancelled.
	// -------------------------------------------------------------------------
	orch := orchestrator.New(cfg.ReconcileInterval, logger)
	go orch.Run(ctx)

	// -------------------------------------------------------------------------
	// HTTP server: run in a separate goroutine.
	// -------------------------------------------------------------------------
	srv := api.New(cfg.HTTPAddr, logger)

	serverErr := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	// -------------------------------------------------------------------------
	// Wait for a shutdown signal or a fatal server error.
	// -------------------------------------------------------------------------
	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-serverErr:
		logger.Error("HTTP server error", "error", err)
		stop() // cancel context so orchestrator exits cleanly
	}

	// -------------------------------------------------------------------------
	// Graceful shutdown: give in-flight HTTP requests time to finish.
	// -------------------------------------------------------------------------
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown error", "error", err)
		os.Exit(1)
	}

	logger.Info("manager stopped")
}

// newLogger constructs a JSON structured logger at the level specified by
// levelStr. Unknown values default to info.
func newLogger(levelStr string) *slog.Logger {
	var level slog.Level
	switch levelStr {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: level}
	handler := slog.NewJSONHandler(os.Stdout, opts)
	return slog.New(handler)
}

// startupDeadline is the time allowed for the service to reach a healthy
// state before the process is considered unhealthy and exits.
const startupDeadline = 10 * time.Second //nolint:unused

package config_test

import (
	"testing"
	"time"

	"github.com/Mewtos7/lx-container-weaver/internal/config"
)

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/weaver")
	// Leave all other vars unset so defaults apply.

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.HTTPAddr != ":8080" {
		t.Errorf("HTTPAddr: want :8080, got %s", cfg.HTTPAddr)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel: want info, got %s", cfg.LogLevel)
	}
	if cfg.ReconcileInterval != 60*time.Second {
		t.Errorf("ReconcileInterval: want 60s, got %v", cfg.ReconcileInterval)
	}
	if cfg.ShutdownTimeout != 30*time.Second {
		t.Errorf("ShutdownTimeout: want 30s, got %v", cfg.ShutdownTimeout)
	}
}

func TestLoad_CustomValues(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://host/db")
	t.Setenv("HTTP_ADDR", ":9090")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("RECONCILE_INTERVAL", "120s")
	t.Setenv("SHUTDOWN_TIMEOUT", "10s")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.HTTPAddr != ":9090" {
		t.Errorf("HTTPAddr: want :9090, got %s", cfg.HTTPAddr)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel: want debug, got %s", cfg.LogLevel)
	}
	if cfg.ReconcileInterval != 120*time.Second {
		t.Errorf("ReconcileInterval: want 120s, got %v", cfg.ReconcileInterval)
	}
	if cfg.ShutdownTimeout != 10*time.Second {
		t.Errorf("ShutdownTimeout: want 10s, got %v", cfg.ShutdownTimeout)
	}
}

func TestLoad_MissingDatabaseURL(t *testing.T) {
	// Ensure DATABASE_URL is not set.
	t.Setenv("DATABASE_URL", "")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected an error when DATABASE_URL is missing, got nil")
	}
}

func TestLoad_InvalidLogLevel(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://host/db")
	t.Setenv("LOG_LEVEL", "verbose")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected an error for invalid LOG_LEVEL, got nil")
	}
}

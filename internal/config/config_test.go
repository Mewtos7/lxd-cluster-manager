package config_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Mewtos7/lx-container-weaver/internal/config"
)

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/weaver")
	t.Setenv("API_KEYS", "$2a$10$placeholder.hash.value.for.testing.purposes.only12345")
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
	t.Setenv("API_KEYS", "hash1,hash2")

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
	if len(cfg.APIKeys) != 2 {
		t.Errorf("APIKeys: want 2 entries, got %d", len(cfg.APIKeys))
	}
}

func TestLoad_MissingDatabaseURL(t *testing.T) {
	// Ensure DATABASE_URL is not set.
	t.Setenv("DATABASE_URL", "")
	t.Setenv("API_KEYS", "somehash")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected an error when DATABASE_URL is missing, got nil")
	}
}

func TestLoad_InvalidLogLevel(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://host/db")
	t.Setenv("LOG_LEVEL", "verbose")
	t.Setenv("API_KEYS", "somehash")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected an error for invalid LOG_LEVEL, got nil")
	}
}

func TestLoad_MissingAPIKeys(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://host/db")
	t.Setenv("API_KEYS", "")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected an error when API_KEYS is missing, got nil")
	}
}

func TestLoad_APIKeysLoaded(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://host/db")
	t.Setenv("API_KEYS", "hash_a,hash_b,hash_c")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(cfg.APIKeys) != 3 {
		t.Errorf("APIKeys: want 3 entries, got %d", len(cfg.APIKeys))
	}
}

func TestLoad_APIKeysWhitespaceTrimmed(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://host/db")
	// Spaces around commas should be tolerated.
	t.Setenv("API_KEYS", "hash_a, hash_b , hash_c")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(cfg.APIKeys) != 3 {
		t.Errorf("APIKeys: want 3 entries after trimming, got %d", len(cfg.APIKeys))
	}
	for i, k := range cfg.APIKeys {
		if k != cfg.APIKeys[i] {
			t.Errorf("APIKeys[%d]: unexpected whitespace", i)
		}
	}
}

func TestLoad_HetznerAPITokenOptional(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://host/db")
	t.Setenv("API_KEYS", "somehash")
	t.Setenv("HETZNER_API_TOKEN", "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("expected no error when HETZNER_API_TOKEN is empty, got %v", err)
	}
	if cfg.HetznerAPIToken != "" {
		t.Errorf("HetznerAPIToken: want empty string, got %q", cfg.HetznerAPIToken)
	}
}

func TestLoad_HetznerAPITokenSet(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://host/db")
	t.Setenv("API_KEYS", "somehash")
	t.Setenv("HETZNER_API_TOKEN", "tok-test-abc")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.HetznerAPIToken != "tok-test-abc" {
		t.Errorf("HetznerAPIToken: want tok-test-abc, got %q", cfg.HetznerAPIToken)
	}
}

func TestLoad_InitialBootstrapEnabled_Default(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://host/db")
	t.Setenv("API_KEYS", "somehash")
	// Leave INITIAL_BOOTSTRAP_ENABLED unset so the default (false) applies.
	t.Setenv("INITIAL_BOOTSTRAP_ENABLED", "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.InitialBootstrapEnabled {
		t.Error("InitialBootstrapEnabled: want false by default, got true")
	}
}

func TestLoad_InitialBootstrapEnabled_True(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://host/db")
	t.Setenv("API_KEYS", "somehash")
	t.Setenv("INITIAL_BOOTSTRAP_ENABLED", "true")
	setBootstrapEnv(t)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !cfg.InitialBootstrapEnabled {
		t.Error("InitialBootstrapEnabled: want true, got false")
	}
}

func TestLoad_InitialBootstrapEnabled_CaseInsensitive(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://host/db")
	t.Setenv("API_KEYS", "somehash")
	t.Setenv("INITIAL_BOOTSTRAP_ENABLED", "TRUE")
	setBootstrapEnv(t)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !cfg.InitialBootstrapEnabled {
		t.Error("InitialBootstrapEnabled: want true for 'TRUE', got false")
	}
}

func TestLoad_InitialBootstrapEnabled_False(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://host/db")
	t.Setenv("API_KEYS", "somehash")
	t.Setenv("INITIAL_BOOTSTRAP_ENABLED", "false")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.InitialBootstrapEnabled {
		t.Error("InitialBootstrapEnabled: want false, got true")
	}
}

// setBootstrapEnv sets all bootstrap environment variables to valid values for
// tests that need INITIAL_BOOTSTRAP_ENABLED=true.
func setBootstrapEnv(t *testing.T) {
	t.Helper()
	t.Setenv("BOOTSTRAP_HYPERSCALER", "hetzner")
	t.Setenv("BOOTSTRAP_REGION", "eu-central")
	t.Setenv("BOOTSTRAP_SERVER_TIER", "low")
}

func TestLoad_Bootstrap_DisabledSkipsValidation(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://host/db")
	t.Setenv("API_KEYS", "somehash")
	t.Setenv("INITIAL_BOOTSTRAP_ENABLED", "false")
	// Leave all BOOTSTRAP_* vars unset — validation must not fail.

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("expected no error when bootstrap is disabled, got %v", err)
	}
	if cfg.Bootstrap.Hyperscaler != "" {
		t.Errorf("Bootstrap.Hyperscaler: want empty, got %q", cfg.Bootstrap.Hyperscaler)
	}
}

func TestLoad_Bootstrap_ValidConfig(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://host/db")
	t.Setenv("API_KEYS", "somehash")
	t.Setenv("INITIAL_BOOTSTRAP_ENABLED", "true")
	setBootstrapEnv(t)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("expected no error for valid bootstrap config, got %v", err)
	}

	b := cfg.Bootstrap
	if b.Hyperscaler != "hetzner" {
		t.Errorf("Bootstrap.Hyperscaler: want %q, got %q", "hetzner", b.Hyperscaler)
	}
	if b.Region != "eu-central" {
		t.Errorf("Bootstrap.Region: want %q, got %q", "eu-central", b.Region)
	}
	if b.ServerTier != config.ServerTierLow {
		t.Errorf("Bootstrap.ServerTier: want %q, got %q", config.ServerTierLow, b.ServerTier)
	}
}

func TestLoad_Bootstrap_MissingFields(t *testing.T) {
	baseSetup := func(t *testing.T) {
		t.Helper()
		t.Setenv("DATABASE_URL", "postgres://host/db")
		t.Setenv("API_KEYS", "somehash")
		t.Setenv("INITIAL_BOOTSTRAP_ENABLED", "true")
		setBootstrapEnv(t)
	}

	tests := []struct {
		name    string
		unset   string
		wantErr string
	}{
		{
			name:    "missing BOOTSTRAP_HYPERSCALER",
			unset:   "BOOTSTRAP_HYPERSCALER",
			wantErr: "BOOTSTRAP_HYPERSCALER is required",
		},
		{
			name:    "missing BOOTSTRAP_REGION",
			unset:   "BOOTSTRAP_REGION",
			wantErr: "BOOTSTRAP_REGION is required",
		},
		{
			name:    "missing BOOTSTRAP_SERVER_TIER",
			unset:   "BOOTSTRAP_SERVER_TIER",
			wantErr: "BOOTSTRAP_SERVER_TIER is required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			baseSetup(t)
			t.Setenv(tc.unset, "")

			_, err := config.Load()
			if err == nil {
				t.Fatalf("expected error when %s is missing, got nil", tc.unset)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestLoad_Bootstrap_InvalidServerTier(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://host/db")
	t.Setenv("API_KEYS", "somehash")
	t.Setenv("INITIAL_BOOTSTRAP_ENABLED", "true")
	setBootstrapEnv(t)
	t.Setenv("BOOTSTRAP_SERVER_TIER", "xlarge")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for invalid BOOTSTRAP_SERVER_TIER, got nil")
	}
	if !strings.Contains(err.Error(), "BOOTSTRAP_SERVER_TIER must be one of low|mid|high") {
		t.Errorf("error %q does not contain expected message", err.Error())
	}
}

func TestLoad_Bootstrap_AllServerTiers(t *testing.T) {
	for _, tier := range []string{"low", "mid", "high"} {
		t.Run(tier, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "postgres://host/db")
			t.Setenv("API_KEYS", "somehash")
			t.Setenv("INITIAL_BOOTSTRAP_ENABLED", "true")
			setBootstrapEnv(t)
			t.Setenv("BOOTSTRAP_SERVER_TIER", tier)

			_, err := config.Load()
			if err != nil {
				t.Fatalf("BOOTSTRAP_SERVER_TIER=%q: unexpected error: %v", tier, err)
			}
		})
	}
}

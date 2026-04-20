// Package config handles loading and validation of runtime configuration.
// All settings are read from environment variables so that the service can be
// configured without a separate config file and follows twelve-factor app
// conventions.
package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// Config holds all runtime settings for the manager service.
type Config struct {
	// HTTPAddr is the address (host:port) the HTTP server binds to.
	// Environment variable: HTTP_ADDR (default: ":8080")
	HTTPAddr string

	// DatabaseURL is the PostgreSQL connection string.
	// Environment variable: DATABASE_URL (required)
	DatabaseURL string

	// LogLevel controls the minimum log level emitted by the structured logger.
	// Accepted values: "debug", "info", "warn", "error".
	// Environment variable: LOG_LEVEL (default: "info")
	LogLevel string

	// ReconcileInterval is how often the orchestration loop runs per cluster.
	// Environment variable: RECONCILE_INTERVAL (default: "60s")
	ReconcileInterval time.Duration

	// ShutdownTimeout is the maximum time the service waits for in-flight
	// requests to complete during graceful shutdown.
	// Environment variable: SHUTDOWN_TIMEOUT (default: "30s")
	ShutdownTimeout time.Duration

	// APIKeys is the list of bcrypt-hashed API keys authorised to call
	// protected endpoints. Each entry is a bcrypt hash of a raw key issued to
	// a client. Configured via the API_KEYS environment variable as a
	// comma-separated list of hashes.
	// Environment variable: API_KEYS (required)
	APIKeys []string

	// HetznerAPIToken is the Hetzner Cloud API token used by the Hetzner
	// provider to authenticate Pulumi operations. Optional: when empty the
	// Hetzner provider is not initialised and the manager runs without a
	// cloud provider.
	// Environment variable: HETZNER_API_TOKEN (optional)
	HetznerAPIToken string

	// InitialBootstrapEnabled controls whether the manager attempts to
	// bootstrap the very first LXD cluster at startup. When false (the
	// default) the bootstrap path is never entered, making startup
	// side-effect free. Set to true only in environments where no cluster
	// exists yet and an automated first-cluster provisioning is desired.
	// Environment variable: INITIAL_BOOTSTRAP_ENABLED (default: false)
	InitialBootstrapEnabled bool

	// Bootstrap holds the operator-supplied settings that drive the very
	// first LXD cluster provisioning. These fields are only loaded and
	// validated when InitialBootstrapEnabled is true. All other bootstrap
	// details (cluster name, trust token, storage config, node names and
	// addresses) are either auto-generated or use hardcoded defaults.
	Bootstrap BootstrapConfig
}

// ServerTier is the abstracted server size used for bootstrap node provisioning.
// It is translated to hyperscaler-specific instance types at provisioning time.
type ServerTier string

const (
	// ServerTierLow is a small server suitable for light workloads.
	ServerTierLow ServerTier = "low"
	// ServerTierMid is a medium-range server for typical production workloads.
	ServerTierMid ServerTier = "mid"
	// ServerTierHigh is a large server for resource-intensive workloads.
	ServerTierHigh ServerTier = "high"
)

// BootstrapConfig holds the minimal operator-supplied settings required to
// provision and bootstrap the first LXD cluster. Hyperscaler-specific
// details (server type, OS image, storage driver/pool, node names and
// addresses, trust token) are resolved from these three inputs or fall back
// to hardcoded defaults, so operators do not need to know provider internals.
//
// All three fields are required when InitialBootstrapEnabled is true; they
// have no effect when bootstrap is disabled.
type BootstrapConfig struct {
	// Hyperscaler identifies the cloud provider used to provision bootstrap
	// nodes (e.g. "hetzner"). The value is matched case-insensitively and
	// translated to the corresponding provider implementation.
	// Environment variable: BOOTSTRAP_HYPERSCALER (required when bootstrap enabled)
	Hyperscaler string

	// Region is a provider-agnostic datacenter region identifier
	// (e.g. "eu-central", "us-east"). It is mapped to the hyperscaler's
	// native region code at provisioning time.
	// Environment variable: BOOTSTRAP_REGION (required when bootstrap enabled)
	Region string

	// ServerTier is the abstracted node size: "low", "mid", or "high".
	// The manager translates this to the hyperscaler's native instance type,
	// so operators do not need to know provider-specific SKU names.
	// Environment variable: BOOTSTRAP_SERVER_TIER (required when bootstrap enabled)
	ServerTier ServerTier
}

// Load reads configuration from environment variables, applies defaults for
// optional settings, and validates that required values are present.
func Load() (*Config, error) {
	cfg := &Config{
		HTTPAddr:                envOr("HTTP_ADDR", ":8080"),
		DatabaseURL:             os.Getenv("DATABASE_URL"),
		LogLevel:                envOr("LOG_LEVEL", "info"),
		ReconcileInterval:       mustParseDuration(envOr("RECONCILE_INTERVAL", "60s")),
		ShutdownTimeout:         mustParseDuration(envOr("SHUTDOWN_TIMEOUT", "30s")),
		APIKeys:                 splitNonEmpty(os.Getenv("API_KEYS"), ","),
		HetznerAPIToken:         os.Getenv("HETZNER_API_TOKEN"),
		InitialBootstrapEnabled: strings.EqualFold(os.Getenv("INITIAL_BOOTSTRAP_ENABLED"), "true"),
		Bootstrap: BootstrapConfig{
			Hyperscaler: os.Getenv("BOOTSTRAP_HYPERSCALER"),
			Region:      os.Getenv("BOOTSTRAP_REGION"),
			ServerTier:  ServerTier(os.Getenv("BOOTSTRAP_SERVER_TIER")),
		},
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// validate checks that all required fields are set and that values are within
// acceptable ranges.
func (c *Config) validate() error {
	var errs []error

	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}

	switch c.LogLevel {
	case "debug", "info", "warn", "error":
		// valid
	default:
		errs = append(errs, fmt.Errorf("LOG_LEVEL must be one of debug|info|warn|error, got %q", c.LogLevel))
	}

	if c.ReconcileInterval <= 0 {
		errs = append(errs, errors.New("RECONCILE_INTERVAL must be a positive duration"))
	}

	if c.ShutdownTimeout <= 0 {
		errs = append(errs, errors.New("SHUTDOWN_TIMEOUT must be a positive duration"))
	}

	if len(c.APIKeys) == 0 {
		errs = append(errs, errors.New("API_KEYS is required: provide at least one bcrypt-hashed API key"))
	}

	if err := c.validateBootstrap(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// validateBootstrap checks that all required bootstrap fields are set when
// InitialBootstrapEnabled is true. When bootstrap is disabled the fields are
// not inspected, so operators can leave them unset in non-bootstrap
// environments.
func (c *Config) validateBootstrap() error {
	if !c.InitialBootstrapEnabled {
		return nil
	}

	b := &c.Bootstrap
	var errs []error

	if b.Hyperscaler == "" {
		errs = append(errs, errors.New("BOOTSTRAP_HYPERSCALER is required when INITIAL_BOOTSTRAP_ENABLED is true"))
	}

	if b.Region == "" {
		errs = append(errs, errors.New("BOOTSTRAP_REGION is required when INITIAL_BOOTSTRAP_ENABLED is true"))
	}

	switch b.ServerTier {
	case ServerTierLow, ServerTierMid, ServerTierHigh:
		// valid
	case "":
		errs = append(errs, errors.New("BOOTSTRAP_SERVER_TIER is required when INITIAL_BOOTSTRAP_ENABLED is true"))
	default:
		errs = append(errs, fmt.Errorf("BOOTSTRAP_SERVER_TIER must be one of low|mid|high, got %q", b.ServerTier))
	}

	return errors.Join(errs...)
}

// envOr returns the value of the environment variable named key, or fallback
// if the variable is unset or empty.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// mustParseDuration parses a duration string and panics if it is invalid.
// It is intended only for parsing default values that are hard-coded literals.
func mustParseDuration(s string) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		panic(fmt.Sprintf("config: invalid duration literal %q: %v", s, err))
	}
	return d
}

// splitNonEmpty splits s by sep, trims whitespace from each token, and returns
// only non-empty tokens. Trimming makes API_KEYS tolerant of values like
// "hash1, hash2" where operators may have added spaces around commas.
func splitNonEmpty(s, sep string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(s, sep) {
		if t := strings.TrimSpace(part); t != "" {
			out = append(out, t)
		}
	}
	return out
}

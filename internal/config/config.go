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

	// Bootstrap holds the settings required to provision and bootstrap the
	// very first LXD cluster. These fields are only loaded and validated
	// when InitialBootstrapEnabled is true.
	Bootstrap BootstrapConfig
}

// BootstrapConfig holds the settings required to provision and bootstrap the
// very first LXD cluster. All fields are required when
// InitialBootstrapEnabled is true; they have no effect when bootstrap is
// disabled.
type BootstrapConfig struct {
	// ClusterName is the human-readable name assigned to the bootstrapped cluster.
	// Environment variable: BOOTSTRAP_CLUSTER_NAME (required when bootstrap enabled)
	ClusterName string

	// HetznerServerType is the Hetzner Cloud server type to provision for
	// each node (e.g. "cx22", "cx32").
	// Environment variable: BOOTSTRAP_HETZNER_SERVER_TYPE (required when bootstrap enabled)
	HetznerServerType string

	// HetznerRegion is the Hetzner Cloud datacenter location for the
	// provisioned servers (e.g. "nbg1", "hel1", "fsn1").
	// Environment variable: BOOTSTRAP_HETZNER_REGION (required when bootstrap enabled)
	HetznerRegion string

	// HetznerImage is the Hetzner Cloud OS image used when creating servers
	// (e.g. "ubuntu-22.04").
	// Environment variable: BOOTSTRAP_HETZNER_IMAGE (required when bootstrap enabled)
	HetznerImage string

	// TrustToken is the shared secret that authorises LXD cluster member
	// joins. It must be the same value on all nodes.
	// Environment variable: BOOTSTRAP_TRUST_TOKEN (required when bootstrap enabled)
	TrustToken string

	// StorageDriver is the LXD storage backend driver
	// (e.g. "dir", "zfs", "btrfs").
	// Environment variable: BOOTSTRAP_STORAGE_DRIVER (required when bootstrap enabled)
	StorageDriver string

	// StoragePool is the name of the LXD storage pool to configure on each
	// node (e.g. "default").
	// Environment variable: BOOTSTRAP_STORAGE_POOL (required when bootstrap enabled)
	StoragePool string

	// SeedNodeName is the LXD cluster member name assigned to the seed
	// (first) node (e.g. "lxd1"). Must be unique within the cluster.
	// Environment variable: BOOTSTRAP_SEED_NODE_NAME (required when bootstrap enabled)
	SeedNodeName string

	// SeedNodeAddress is the host:port address on which the seed node
	// listens for cluster member connections (e.g. "10.0.0.1:8443").
	// Environment variable: BOOTSTRAP_SEED_NODE_ADDRESS (required when bootstrap enabled)
	SeedNodeAddress string

	// JoinerNodeName is the LXD cluster member name assigned to the joiner
	// (second) node (e.g. "lxd2"). Must be unique within the cluster.
	// Environment variable: BOOTSTRAP_JOINER_NODE_NAME (required when bootstrap enabled)
	JoinerNodeName string

	// JoinerNodeAddress is the host:port address on which the joiner node
	// listens for cluster member connections (e.g. "10.0.0.2:8443").
	// Environment variable: BOOTSTRAP_JOINER_NODE_ADDRESS (required when bootstrap enabled)
	JoinerNodeAddress string
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
			ClusterName:       os.Getenv("BOOTSTRAP_CLUSTER_NAME"),
			HetznerServerType: os.Getenv("BOOTSTRAP_HETZNER_SERVER_TYPE"),
			HetznerRegion:     os.Getenv("BOOTSTRAP_HETZNER_REGION"),
			HetznerImage:      os.Getenv("BOOTSTRAP_HETZNER_IMAGE"),
			TrustToken:        os.Getenv("BOOTSTRAP_TRUST_TOKEN"),
			StorageDriver:     os.Getenv("BOOTSTRAP_STORAGE_DRIVER"),
			StoragePool:       os.Getenv("BOOTSTRAP_STORAGE_POOL"),
			SeedNodeName:      os.Getenv("BOOTSTRAP_SEED_NODE_NAME"),
			SeedNodeAddress:   os.Getenv("BOOTSTRAP_SEED_NODE_ADDRESS"),
			JoinerNodeName:    os.Getenv("BOOTSTRAP_JOINER_NODE_NAME"),
			JoinerNodeAddress: os.Getenv("BOOTSTRAP_JOINER_NODE_ADDRESS"),
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

	if b.ClusterName == "" {
		errs = append(errs, errors.New("BOOTSTRAP_CLUSTER_NAME is required when INITIAL_BOOTSTRAP_ENABLED is true"))
	}
	if b.HetznerServerType == "" {
		errs = append(errs, errors.New("BOOTSTRAP_HETZNER_SERVER_TYPE is required when INITIAL_BOOTSTRAP_ENABLED is true"))
	}
	if b.HetznerRegion == "" {
		errs = append(errs, errors.New("BOOTSTRAP_HETZNER_REGION is required when INITIAL_BOOTSTRAP_ENABLED is true"))
	}
	if b.HetznerImage == "" {
		errs = append(errs, errors.New("BOOTSTRAP_HETZNER_IMAGE is required when INITIAL_BOOTSTRAP_ENABLED is true"))
	}
	if b.TrustToken == "" {
		errs = append(errs, errors.New("BOOTSTRAP_TRUST_TOKEN is required when INITIAL_BOOTSTRAP_ENABLED is true"))
	}
	if b.StorageDriver == "" {
		errs = append(errs, errors.New("BOOTSTRAP_STORAGE_DRIVER is required when INITIAL_BOOTSTRAP_ENABLED is true"))
	}
	if b.StoragePool == "" {
		errs = append(errs, errors.New("BOOTSTRAP_STORAGE_POOL is required when INITIAL_BOOTSTRAP_ENABLED is true"))
	}
	if b.SeedNodeName == "" {
		errs = append(errs, errors.New("BOOTSTRAP_SEED_NODE_NAME is required when INITIAL_BOOTSTRAP_ENABLED is true"))
	}
	if b.SeedNodeAddress == "" {
		errs = append(errs, errors.New("BOOTSTRAP_SEED_NODE_ADDRESS is required when INITIAL_BOOTSTRAP_ENABLED is true"))
	}
	if b.JoinerNodeName == "" {
		errs = append(errs, errors.New("BOOTSTRAP_JOINER_NODE_NAME is required when INITIAL_BOOTSTRAP_ENABLED is true"))
	}
	if b.JoinerNodeAddress == "" {
		errs = append(errs, errors.New("BOOTSTRAP_JOINER_NODE_ADDRESS is required when INITIAL_BOOTSTRAP_ENABLED is true"))
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

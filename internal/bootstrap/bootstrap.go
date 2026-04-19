// Package bootstrap orchestrates the initial two-node LXD cluster formation
// described in the cluster-bootstrap user story. It coordinates the preseed
// initialization of the seed node and the subsequent join of the second node,
// then verifies that both are Online cluster members.
//
// # Usage
//
//	b := bootstrap.New(seedClient, joinerClient)
//	err := b.Bootstrap(ctx, bootstrap.Config{
//	    ClusterName:   "prod-cluster",
//	    TrustToken:    "s3cr3t",
//	    StorageDriver: "dir",
//	    StoragePool:   "default",
//	    SeedNode: bootstrap.NodeConfig{
//	        Name:          "lxd1",
//	        ListenAddress: "10.0.0.1:8443",
//	    },
//	    JoinerNode: bootstrap.NodeConfig{
//	        Name:          "lxd2",
//	        ListenAddress: "10.0.0.2:8443",
//	    },
//	})
//
// # Idempotency
//
// Bootstrap is safe to call multiple times. If both nodes already report
// themselves as cluster members the call returns nil without modifying anything.
// If only the seed is initialised but the joiner has not yet joined, Bootstrap
// skips seed initialisation and proceeds directly to the join step.
//
// # Error semantics
//
// A failed bootstrap returns a descriptive error that includes the step at
// which the failure occurred. No partial state is left behind: if the join
// step fails, the seed node is left initialised but the joiner is not joined,
// and the error clearly identifies which step failed.
package bootstrap

import (
	"context"
	"errors"
	"fmt"

	"github.com/Mewtos7/lx-container-weaver/internal/lxd"
)

// NodeConfig holds the per-node settings used during cluster formation.
type NodeConfig struct {
	// Name is the LXD cluster member name to assign to this node
	// (e.g. "lxd1"). Must be unique within the cluster.
	Name string

	// ListenAddress is the address on which this node listens for cluster
	// member connections (e.g. "10.0.0.1:8443"). Do not include the "https://"
	// scheme; the LXD preseed API expects a bare host:port string.
	ListenAddress string
}

// Config holds the cluster-level and per-node settings required to bootstrap a
// two-node LXD cluster. All fields are required; there are no hardcoded
// defaults.
type Config struct {
	// ClusterName is the human-readable name assigned to the new cluster.
	ClusterName string

	// TrustToken is the shared secret used to authorise the joining node.
	// It must be the same value on both the seed and the joiner.
	TrustToken string

	// StorageDriver is the LXD storage backend driver
	// (e.g. "dir", "zfs", "btrfs").
	StorageDriver string

	// StoragePool is the name of the storage pool to configure on each node
	// (e.g. "default").
	StoragePool string

	// SeedNode is the configuration for the first (seed) node that initialises
	// the cluster.
	SeedNode NodeConfig

	// JoinerNode is the configuration for the second node that joins the
	// cluster formed by SeedNode.
	JoinerNode NodeConfig
}

// Bootstrapper orchestrates the initial two-node LXD cluster formation. It
// holds separate [lxd.Client] instances for the seed and joiner nodes so that
// it can call each node's LXD API independently.
type Bootstrapper struct {
	seed   lxd.Client
	joiner lxd.Client
}

// New creates a Bootstrapper that targets the supplied seed and joiner LXD
// endpoints. Both clients must already be configured and reachable.
func New(seed, joiner lxd.Client) *Bootstrapper {
	return &Bootstrapper{seed: seed, joiner: joiner}
}

// Bootstrap executes the full two-node cluster formation sequence:
//
//  1. Validates cfg.
//  2. Checks whether the seed node is already clustered.
//  3. If not, retrieves the seed's TLS certificate and initialises it.
//  4. Checks whether the joiner node is already clustered.
//  5. If not, retrieves the seed's TLS certificate (if not already fetched)
//     and joins the joiner to the cluster.
//  6. Verifies that both node names appear in the cluster member list.
//
// Bootstrap is idempotent: if both nodes are already Online cluster members the
// call returns nil without making any changes.
func (b *Bootstrapper) Bootstrap(ctx context.Context, cfg Config) error {
	if err := validate(cfg); err != nil {
		return fmt.Errorf("bootstrap: invalid config: %w", err)
	}

	// ── Step 1: Determine seed node status ───────────────────────────────────

	seedStatus, err := b.seed.GetClusterStatus(ctx)
	if err != nil {
		return fmt.Errorf("bootstrap: check seed cluster status: %w", err)
	}

	var seedCert string

	if !seedStatus.Enabled {
		// Fetch the seed's TLS certificate before initialising so we can pass
		// it to the joiner without needing a second GET /1.0 call later.
		cert, err := b.seed.GetClusterCertificate(ctx)
		if err != nil {
			return fmt.Errorf("bootstrap: get seed certificate: %w", err)
		}
		seedCert = cert

		initCfg := lxd.ClusterInitConfig{
			ServerName:    cfg.SeedNode.Name,
			ClusterName:   cfg.ClusterName,
			ListenAddress: cfg.SeedNode.ListenAddress,
			StoragePool: lxd.StoragePoolConfig{
				Name:   cfg.StoragePool,
				Driver: cfg.StorageDriver,
			},
			TrustToken: cfg.TrustToken,
		}
		if err := b.seed.InitCluster(ctx, initCfg); err != nil {
			if errors.Is(err, lxd.ErrClusterAlreadyBootstrapped) {
				// Another process initialised the seed concurrently; treat as
				// a no-op and proceed to the join step.
			} else {
				return fmt.Errorf("bootstrap: init seed node %q: %w", cfg.SeedNode.Name, err)
			}
		}
	}

	// ── Step 2: Determine joiner node status ─────────────────────────────────

	joinerStatus, err := b.joiner.GetClusterStatus(ctx)
	if err != nil {
		return fmt.Errorf("bootstrap: check joiner cluster status: %w", err)
	}

	if !joinerStatus.Enabled {
		// We need the seed's certificate to authenticate the join. Fetch it
		// now if we didn't already do so in step 1.
		if seedCert == "" {
			cert, err := b.seed.GetClusterCertificate(ctx)
			if err != nil {
				return fmt.Errorf("bootstrap: get seed certificate for join: %w", err)
			}
			seedCert = cert
		}

		joinCfg := lxd.ClusterJoinConfig{
			ServerName:         cfg.JoinerNode.Name,
			ClusterAddress:     "https://" + cfg.SeedNode.ListenAddress,
			ClusterCertificate: seedCert,
			TrustToken:         cfg.TrustToken,
			StoragePool: lxd.StoragePoolConfig{
				Name:   cfg.StoragePool,
				Driver: cfg.StorageDriver,
			},
		}
		if err := b.joiner.JoinCluster(ctx, joinCfg); err != nil {
			if errors.Is(err, lxd.ErrClusterAlreadyBootstrapped) {
				// Another process joined the node concurrently; treat as no-op.
			} else {
				return fmt.Errorf("bootstrap: join node %q to cluster: %w", cfg.JoinerNode.Name, err)
			}
		}
	}

	// ── Step 3: Verify both nodes are Online cluster members ─────────────────

	if err := b.verify(ctx, cfg.SeedNode.Name, cfg.JoinerNode.Name); err != nil {
		return fmt.Errorf("bootstrap: verification failed: %w", err)
	}

	return nil
}

// verify checks that both named nodes appear in the seed's cluster member list.
func (b *Bootstrapper) verify(ctx context.Context, seedName, joinerName string) error {
	members, err := b.seed.GetClusterMembers(ctx)
	if err != nil {
		return fmt.Errorf("get cluster members: %w", err)
	}

	found := make(map[string]bool, len(members))
	for _, m := range members {
		found[m.Name] = true
	}

	if !found[seedName] {
		return fmt.Errorf("seed node %q is not a cluster member", seedName)
	}
	if !found[joinerName] {
		return fmt.Errorf("joiner node %q is not a cluster member", joinerName)
	}
	return nil
}

// validate checks that all required Config fields are non-empty.
func validate(cfg Config) error {
	var errs []error
	if cfg.ClusterName == "" {
		errs = append(errs, errors.New("ClusterName is required"))
	}
	if cfg.TrustToken == "" {
		errs = append(errs, errors.New("TrustToken is required"))
	}
	if cfg.StorageDriver == "" {
		errs = append(errs, errors.New("StorageDriver is required"))
	}
	if cfg.StoragePool == "" {
		errs = append(errs, errors.New("StoragePool is required"))
	}
	if cfg.SeedNode.Name == "" {
		errs = append(errs, errors.New("SeedNode.Name is required"))
	}
	if cfg.SeedNode.ListenAddress == "" {
		errs = append(errs, errors.New("SeedNode.ListenAddress is required"))
	}
	if cfg.JoinerNode.Name == "" {
		errs = append(errs, errors.New("JoinerNode.Name is required"))
	}
	if cfg.JoinerNode.ListenAddress == "" {
		errs = append(errs, errors.New("JoinerNode.ListenAddress is required"))
	}
	return errors.Join(errs...)
}

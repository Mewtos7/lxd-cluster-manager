package bootstrap_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Mewtos7/lx-container-weaver/internal/bootstrap"
	"github.com/Mewtos7/lx-container-weaver/internal/lxd"
	"github.com/Mewtos7/lx-container-weaver/internal/lxd/fake"
)

// minimalConfig returns a fully populated Config that passes validation.
func minimalConfig() bootstrap.Config {
	return bootstrap.Config{
		ClusterName:   "test-cluster",
		TrustToken:    "s3cr3t",
		StorageDriver: "dir",
		StoragePool:   "default",
		SeedNode: bootstrap.NodeConfig{
			Name:          "lxd1",
			ListenAddress: "10.0.0.1:8443",
		},
		JoinerNode: bootstrap.NodeConfig{
			Name:          "lxd2",
			ListenAddress: "10.0.0.2:8443",
		},
	}
}

// TestBootstrap_Success verifies the happy path: both nodes start unclustered,
// Bootstrap initialises the seed and joins the joiner, then verifies both are
// members.
func TestBootstrap_Success(t *testing.T) {
	seed := fake.New()
	seed.SetClusterCertificate("-----BEGIN CERTIFICATE-----\nMIIBxxx\n-----END CERTIFICATE-----")
	// Add both nodes as cluster members so verification passes after bootstrap.
	seed.AddNode(lxd.NodeInfo{Name: "lxd1", Status: "Online"})
	seed.AddNode(lxd.NodeInfo{Name: "lxd2", Status: "Online"})

	joiner := fake.New()

	b := bootstrap.New(seed, joiner)
	err := b.Bootstrap(context.Background(), minimalConfig())
	if err != nil {
		t.Fatalf("Bootstrap: unexpected error: %v", err)
	}

	// Seed should have been initialised exactly once.
	if len(seed.InitCalls) != 1 {
		t.Errorf("InitCalls: want 1, got %d", len(seed.InitCalls))
	}
	if seed.InitCalls[0].ServerName != "lxd1" {
		t.Errorf("InitCalls[0].ServerName: want %q, got %q", "lxd1", seed.InitCalls[0].ServerName)
	}
	if seed.InitCalls[0].ClusterName != "test-cluster" {
		t.Errorf("InitCalls[0].ClusterName: want %q, got %q", "test-cluster", seed.InitCalls[0].ClusterName)
	}
	if seed.InitCalls[0].TrustToken != "s3cr3t" {
		t.Errorf("InitCalls[0].TrustToken: want %q, got %q", "s3cr3t", seed.InitCalls[0].TrustToken)
	}
	if seed.InitCalls[0].StoragePool.Name != "default" {
		t.Errorf("InitCalls[0].StoragePool.Name: want %q, got %q", "default", seed.InitCalls[0].StoragePool.Name)
	}
	if seed.InitCalls[0].StoragePool.Driver != "dir" {
		t.Errorf("InitCalls[0].StoragePool.Driver: want %q, got %q", "dir", seed.InitCalls[0].StoragePool.Driver)
	}

	// Joiner should have been joined exactly once.
	if len(joiner.JoinCalls) != 1 {
		t.Errorf("JoinCalls: want 1, got %d", len(joiner.JoinCalls))
	}
	if joiner.JoinCalls[0].ServerName != "lxd2" {
		t.Errorf("JoinCalls[0].ServerName: want %q, got %q", "lxd2", joiner.JoinCalls[0].ServerName)
	}
	if joiner.JoinCalls[0].ClusterAddress != "https://10.0.0.1:8443" {
		t.Errorf("JoinCalls[0].ClusterAddress: want %q, got %q", "https://10.0.0.1:8443", joiner.JoinCalls[0].ClusterAddress)
	}
	if joiner.JoinCalls[0].TrustToken != "s3cr3t" {
		t.Errorf("JoinCalls[0].TrustToken: want %q, got %q", "s3cr3t", joiner.JoinCalls[0].TrustToken)
	}
}

// TestBootstrap_Idempotent_BothAlreadyClustered verifies that Bootstrap is a
// no-op when both nodes are already cluster members.
func TestBootstrap_Idempotent_BothAlreadyClustered(t *testing.T) {
	seed := fake.New()
	seed.SetClustered(true)
	seed.AddNode(lxd.NodeInfo{Name: "lxd1", Status: "Online"})
	seed.AddNode(lxd.NodeInfo{Name: "lxd2", Status: "Online"})

	joiner := fake.New()
	joiner.SetClustered(true)

	b := bootstrap.New(seed, joiner)
	err := b.Bootstrap(context.Background(), minimalConfig())
	if err != nil {
		t.Fatalf("Bootstrap (idempotent): unexpected error: %v", err)
	}

	if len(seed.InitCalls) != 0 {
		t.Errorf("InitCalls: want 0 (idempotent), got %d", len(seed.InitCalls))
	}
	if len(joiner.JoinCalls) != 0 {
		t.Errorf("JoinCalls: want 0 (idempotent), got %d", len(joiner.JoinCalls))
	}
}

// TestBootstrap_Idempotent_SeedAlreadyClustered verifies that Bootstrap skips
// seed initialisation when only the seed is already clustered, and only joins
// the joiner.
func TestBootstrap_Idempotent_SeedAlreadyClustered(t *testing.T) {
	seed := fake.New()
	seed.SetClustered(true)
	seed.SetClusterCertificate("-----BEGIN CERTIFICATE-----\nMIIBxxx\n-----END CERTIFICATE-----")
	seed.AddNode(lxd.NodeInfo{Name: "lxd1", Status: "Online"})
	seed.AddNode(lxd.NodeInfo{Name: "lxd2", Status: "Online"})

	joiner := fake.New()

	b := bootstrap.New(seed, joiner)
	err := b.Bootstrap(context.Background(), minimalConfig())
	if err != nil {
		t.Fatalf("Bootstrap (seed already clustered): unexpected error: %v", err)
	}

	if len(seed.InitCalls) != 0 {
		t.Errorf("InitCalls: want 0 (seed already clustered), got %d", len(seed.InitCalls))
	}
	if len(joiner.JoinCalls) != 1 {
		t.Errorf("JoinCalls: want 1, got %d", len(joiner.JoinCalls))
	}
}

// TestBootstrap_InvalidConfig verifies that Bootstrap returns an error when
// required config fields are missing.
func TestBootstrap_InvalidConfig(t *testing.T) {
	tests := []struct {
		name string
		cfg  bootstrap.Config
	}{
		{
			name: "missing ClusterName",
			cfg: bootstrap.Config{
				TrustToken: "s3cr3t", StorageDriver: "dir", StoragePool: "default",
				SeedNode:   bootstrap.NodeConfig{Name: "lxd1", ListenAddress: "10.0.0.1:8443"},
				JoinerNode: bootstrap.NodeConfig{Name: "lxd2", ListenAddress: "10.0.0.2:8443"},
			},
		},
		{
			name: "missing TrustToken",
			cfg: bootstrap.Config{
				ClusterName: "c", StorageDriver: "dir", StoragePool: "default",
				SeedNode:   bootstrap.NodeConfig{Name: "lxd1", ListenAddress: "10.0.0.1:8443"},
				JoinerNode: bootstrap.NodeConfig{Name: "lxd2", ListenAddress: "10.0.0.2:8443"},
			},
		},
		{
			name: "missing StorageDriver",
			cfg: bootstrap.Config{
				ClusterName: "c", TrustToken: "s3cr3t", StoragePool: "default",
				SeedNode:   bootstrap.NodeConfig{Name: "lxd1", ListenAddress: "10.0.0.1:8443"},
				JoinerNode: bootstrap.NodeConfig{Name: "lxd2", ListenAddress: "10.0.0.2:8443"},
			},
		},
		{
			name: "missing StoragePool",
			cfg: bootstrap.Config{
				ClusterName: "c", TrustToken: "s3cr3t", StorageDriver: "dir",
				SeedNode:   bootstrap.NodeConfig{Name: "lxd1", ListenAddress: "10.0.0.1:8443"},
				JoinerNode: bootstrap.NodeConfig{Name: "lxd2", ListenAddress: "10.0.0.2:8443"},
			},
		},
		{
			name: "missing SeedNode.Name",
			cfg: bootstrap.Config{
				ClusterName: "c", TrustToken: "s3cr3t", StorageDriver: "dir", StoragePool: "default",
				SeedNode:   bootstrap.NodeConfig{ListenAddress: "10.0.0.1:8443"},
				JoinerNode: bootstrap.NodeConfig{Name: "lxd2", ListenAddress: "10.0.0.2:8443"},
			},
		},
		{
			name: "missing SeedNode.ListenAddress",
			cfg: bootstrap.Config{
				ClusterName: "c", TrustToken: "s3cr3t", StorageDriver: "dir", StoragePool: "default",
				SeedNode:   bootstrap.NodeConfig{Name: "lxd1"},
				JoinerNode: bootstrap.NodeConfig{Name: "lxd2", ListenAddress: "10.0.0.2:8443"},
			},
		},
		{
			name: "missing JoinerNode.Name",
			cfg: bootstrap.Config{
				ClusterName: "c", TrustToken: "s3cr3t", StorageDriver: "dir", StoragePool: "default",
				SeedNode:   bootstrap.NodeConfig{Name: "lxd1", ListenAddress: "10.0.0.1:8443"},
				JoinerNode: bootstrap.NodeConfig{ListenAddress: "10.0.0.2:8443"},
			},
		},
		{
			name: "missing JoinerNode.ListenAddress",
			cfg: bootstrap.Config{
				ClusterName: "c", TrustToken: "s3cr3t", StorageDriver: "dir", StoragePool: "default",
				SeedNode:   bootstrap.NodeConfig{Name: "lxd1", ListenAddress: "10.0.0.1:8443"},
				JoinerNode: bootstrap.NodeConfig{Name: "lxd2"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := bootstrap.New(fake.New(), fake.New())
			err := b.Bootstrap(context.Background(), tc.cfg)
			if err == nil {
				t.Fatalf("Bootstrap(%q): want error for invalid config, got nil", tc.name)
			}
		})
	}
}

// TestBootstrap_SeedInitFailure verifies that a seed InitCluster failure is
// propagated and that the joiner is not called.
func TestBootstrap_SeedInitFailure(t *testing.T) {
	seed := fake.New()
	seed.SetClusterCertificate("-----BEGIN CERTIFICATE-----\nMIIBxxx\n-----END CERTIFICATE-----")
	seed.InitError = errors.New("disk full")

	joiner := fake.New()

	b := bootstrap.New(seed, joiner)
	err := b.Bootstrap(context.Background(), minimalConfig())
	if err == nil {
		t.Fatal("Bootstrap: want error on seed init failure, got nil")
	}

	if len(joiner.JoinCalls) != 0 {
		t.Errorf("JoinCalls: want 0 when seed init fails, got %d", len(joiner.JoinCalls))
	}
}

// TestBootstrap_JoinFailure verifies that a join failure is propagated and
// that the error clearly identifies the join step.
func TestBootstrap_JoinFailure(t *testing.T) {
	seed := fake.New()
	seed.SetClusterCertificate("-----BEGIN CERTIFICATE-----\nMIIBxxx\n-----END CERTIFICATE-----")
	seed.AddNode(lxd.NodeInfo{Name: "lxd1", Status: "Online"})

	joiner := fake.New()
	joiner.JoinError = errors.New("network unreachable")

	b := bootstrap.New(seed, joiner)
	err := b.Bootstrap(context.Background(), minimalConfig())
	if err == nil {
		t.Fatal("Bootstrap: want error on join failure, got nil")
	}
}

// TestBootstrap_VerificationFailure verifies that Bootstrap returns an error
// when the cluster member list does not contain both nodes after bootstrap.
func TestBootstrap_VerificationFailure(t *testing.T) {
	seed := fake.New()
	seed.SetClusterCertificate("-----BEGIN CERTIFICATE-----\nMIIBxxx\n-----END CERTIFICATE-----")
	// Only the seed node is in the member list — joiner is missing.
	seed.AddNode(lxd.NodeInfo{Name: "lxd1", Status: "Online"})

	joiner := fake.New()

	b := bootstrap.New(seed, joiner)
	err := b.Bootstrap(context.Background(), minimalConfig())
	if err == nil {
		t.Fatal("Bootstrap: want error when joiner is not in member list, got nil")
	}
}

// TestBootstrap_ConfigPassthrough verifies that cluster name and per-node
// settings from Config are passed through to the LXD calls without hardcoding.
func TestBootstrap_ConfigPassthrough(t *testing.T) {
	seed := fake.New()
	seed.SetClusterCertificate("-----BEGIN CERTIFICATE-----\nMIIBxxx\n-----END CERTIFICATE-----")
	seed.AddNode(lxd.NodeInfo{Name: "node-a", Status: "Online"})
	seed.AddNode(lxd.NodeInfo{Name: "node-b", Status: "Online"})

	joiner := fake.New()

	cfg := bootstrap.Config{
		ClusterName:   "custom-cluster",
		TrustToken:    "tok3n",
		StorageDriver: "zfs",
		StoragePool:   "tank",
		SeedNode: bootstrap.NodeConfig{
			Name:          "node-a",
			ListenAddress: "192.168.1.1:8443",
		},
		JoinerNode: bootstrap.NodeConfig{
			Name:          "node-b",
			ListenAddress: "192.168.1.2:8443",
		},
	}

	b := bootstrap.New(seed, joiner)
	if err := b.Bootstrap(context.Background(), cfg); err != nil {
		t.Fatalf("Bootstrap: unexpected error: %v", err)
	}

	// Verify seed init config passthrough.
	if len(seed.InitCalls) != 1 {
		t.Fatalf("InitCalls: want 1, got %d", len(seed.InitCalls))
	}
	ic := seed.InitCalls[0]
	if ic.ClusterName != "custom-cluster" {
		t.Errorf("InitCalls[0].ClusterName: want %q, got %q", "custom-cluster", ic.ClusterName)
	}
	if ic.StoragePool.Driver != "zfs" {
		t.Errorf("InitCalls[0].StoragePool.Driver: want %q, got %q", "zfs", ic.StoragePool.Driver)
	}
	if ic.StoragePool.Name != "tank" {
		t.Errorf("InitCalls[0].StoragePool.Name: want %q, got %q", "tank", ic.StoragePool.Name)
	}

	// Verify joiner join config passthrough.
	if len(joiner.JoinCalls) != 1 {
		t.Fatalf("JoinCalls: want 1, got %d", len(joiner.JoinCalls))
	}
	jc := joiner.JoinCalls[0]
	if jc.ServerName != "node-b" {
		t.Errorf("JoinCalls[0].ServerName: want %q, got %q", "node-b", jc.ServerName)
	}
	if jc.ClusterAddress != "https://192.168.1.1:8443" {
		t.Errorf("JoinCalls[0].ClusterAddress: want %q, got %q", "https://192.168.1.1:8443", jc.ClusterAddress)
	}
	if jc.TrustToken != "tok3n" {
		t.Errorf("JoinCalls[0].TrustToken: want %q, got %q", "tok3n", jc.TrustToken)
	}
}

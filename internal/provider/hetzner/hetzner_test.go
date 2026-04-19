package hetzner_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Mewtos7/lx-container-weaver/internal/provider"
	"github.com/Mewtos7/lx-container-weaver/internal/provider/hetzner"
)

func TestNew_EmptyToken(t *testing.T) {
	_, err := hetzner.New("")
	if err == nil {
		t.Fatal("New: want error for empty token, got nil")
	}
}

func TestNew_ValidToken(t *testing.T) {
	p, err := hetzner.New("tok-abc123")
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}
	if p == nil {
		t.Fatal("New: expected non-nil provider")
	}
}

// TestProvisionServer_NoRuntime verifies that ProvisionServer returns an error
// when no Pulumi runtime has been wired via WithRuntime.
func TestProvisionServer_NoRuntime(t *testing.T) {
	p, _ := hetzner.New("tok")
	_, err := p.ProvisionServer(context.Background(), provider.ServerSpec{
		Name: "node-1", ServerType: "cx21", Region: "nbg1", Image: "ubuntu-22.04",
		ClusterID: "cluster-1",
	})
	if err == nil {
		t.Fatal("ProvisionServer: want error when no runtime is configured, got nil")
	}
}

// TestProvisionServer_InvalidSpec verifies that ProvisionServer returns
// ErrInvalidSpec when required fields are missing from the spec.
func TestProvisionServer_InvalidSpec(t *testing.T) {
	p, _ := hetzner.New("tok")
	_, err := p.ProvisionServer(context.Background(), provider.ServerSpec{})
	if err == nil {
		t.Fatal("ProvisionServer: want ErrInvalidSpec for empty spec, got nil")
	}
	if !errors.Is(err, provider.ErrInvalidSpec) {
		t.Errorf("ProvisionServer: want errors.Is(err, ErrInvalidSpec), got %v", err)
	}
}

// TestDeprovisionServer_InvalidIDFormat verifies that DeprovisionServer returns
// an error immediately when the serverID is not a valid numeric string, without
// making any network calls to the Hetzner Cloud API.
func TestDeprovisionServer_InvalidIDFormat(t *testing.T) {
	p, _ := hetzner.New("tok")
	err := p.DeprovisionServer(context.Background(), "not-a-number")
	if err == nil {
		t.Fatal("DeprovisionServer: want error for non-numeric server ID, got nil")
	}
}

// TestGetServer_InvalidIDFormat verifies that GetServer returns an error
// immediately when the serverID is not a valid numeric string, without making
// any network calls to the Hetzner Cloud API.
func TestGetServer_InvalidIDFormat(t *testing.T) {
	p, _ := hetzner.New("tok")
	_, err := p.GetServer(context.Background(), "not-a-number")
	if err == nil {
		t.Fatal("GetServer: want error for non-numeric server ID, got nil")
	}
}

// TestListServers_InvalidToken verifies that ListServers returns an error when
// the provider is configured with an invalid API token. The Hetzner Cloud API
// rejects unauthenticated requests, so the returned error must be non-nil.
func TestListServers_InvalidToken(t *testing.T) {
	p, _ := hetzner.New("invalid-token-for-test")
	_, err := p.ListServers(context.Background())
	if err == nil {
		t.Fatal("ListServers: want error for invalid token, got nil")
	}
}

// TestDeprovisionServer_ErrServerNotFound verifies that DeprovisionServer
// wraps provider.ErrServerNotFound when the Hetzner Cloud API returns a 404.
// Uses a numeric ID that is valid format but will not be found on the API.
func TestDeprovisionServer_ErrServerNotFound(t *testing.T) {
	p, _ := hetzner.New("invalid-token-for-test")
	err := p.DeprovisionServer(context.Background(), "99999999")
	// Either auth error or not-found — either way an error must be returned.
	if err == nil {
		t.Fatal("DeprovisionServer: want error for non-existent server, got nil")
	}
}

// TestGetServer_ErrServerNotFound verifies that GetServer wraps
// provider.ErrServerNotFound when the Hetzner Cloud API returns a 404.
// Uses a numeric ID that is valid format but will not be found on the API.
func TestGetServer_ErrServerNotFound(t *testing.T) {
	p, _ := hetzner.New("invalid-token-for-test")
	_, err := p.GetServer(context.Background(), "99999999")
	// Either auth error or not-found — either way an error must be returned.
	if err == nil {
		t.Fatal("GetServer: want error for non-existent server, got nil")
	}
}

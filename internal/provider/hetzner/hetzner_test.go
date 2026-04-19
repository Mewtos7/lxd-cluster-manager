package hetzner_test

import (
	"context"
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

func TestProvisionServer_ReturnsError(t *testing.T) {
	p, _ := hetzner.New("tok")
	_, err := p.ProvisionServer(context.Background(), provider.ServerSpec{
		Name: "node-1", ServerType: "cx21", Region: "nbg1", Image: "ubuntu-22.04",
	})
	if err == nil {
		t.Fatal("ProvisionServer: want error from stub, got nil")
	}
}

func TestDeprovisionServer_ReturnsError(t *testing.T) {
	p, _ := hetzner.New("tok")
	err := p.DeprovisionServer(context.Background(), "server-42")
	if err == nil {
		t.Fatal("DeprovisionServer: want error from stub, got nil")
	}
}

func TestGetServer_ReturnsError(t *testing.T) {
	p, _ := hetzner.New("tok")
	_, err := p.GetServer(context.Background(), "server-42")
	if err == nil {
		t.Fatal("GetServer: want error from stub, got nil")
	}
}

func TestListServers_ReturnsError(t *testing.T) {
	p, _ := hetzner.New("tok")
	_, err := p.ListServers(context.Background())
	if err == nil {
		t.Fatal("ListServers: want error from stub, got nil")
	}
}

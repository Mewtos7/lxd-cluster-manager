package provider_test

import (
	"errors"
	"testing"

	"github.com/Mewtos7/lx-container-weaver/internal/provider"
	"github.com/Mewtos7/lx-container-weaver/internal/provider/hetzner"
)

// Compile-time assertion: hetzner.Provider must satisfy HyperscalerProvider.
var _ provider.HyperscalerProvider = (*hetzner.Provider)(nil)

// TestSentinelErrors verifies that the exported error values are distinct and
// can be matched with errors.Is after wrapping.
func TestSentinelErrors(t *testing.T) {
	t.Run("ErrServerNotFound is distinct from ErrInvalidSpec", func(t *testing.T) {
		if errors.Is(provider.ErrServerNotFound, provider.ErrInvalidSpec) {
			t.Error("ErrServerNotFound must not match ErrInvalidSpec")
		}
		if errors.Is(provider.ErrInvalidSpec, provider.ErrServerNotFound) {
			t.Error("ErrInvalidSpec must not match ErrServerNotFound")
		}
	})

	t.Run("ErrServerNotFound survives wrapping", func(t *testing.T) {
		wrapped := errors.Join(errors.New("outer"), provider.ErrServerNotFound)
		if !errors.Is(wrapped, provider.ErrServerNotFound) {
			t.Error("wrapped error must satisfy errors.Is(ErrServerNotFound)")
		}
	})

	t.Run("ErrInvalidSpec survives wrapping", func(t *testing.T) {
		wrapped := errors.Join(errors.New("outer"), provider.ErrInvalidSpec)
		if !errors.Is(wrapped, provider.ErrInvalidSpec) {
			t.Error("wrapped error must satisfy errors.Is(ErrInvalidSpec)")
		}
	})
}

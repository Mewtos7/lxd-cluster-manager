package memory

import (
	"context"
	"sync"

	"github.com/Mewtos7/lx-container-weaver/internal/persistence"
)

// compile-time interface check.
var _ persistence.BootstrapLocker = (*Lock)(nil)

// Lock is a thread-safe in-memory implementation of persistence.BootstrapLocker
// intended for use in unit tests where a real database is not available.
type Lock struct {
	mu     sync.Mutex
	locked bool
}

// NewLock returns an unlocked Lock.
func NewLock() *Lock {
	return &Lock{}
}

// TryLock acquires the lock without blocking. Returns (true, nil) when the
// lock is free and is now held by the caller, and (false, nil) when another
// caller already holds it.
func (l *Lock) TryLock(_ context.Context) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.locked {
		return false, nil
	}
	l.locked = true
	return true, nil
}

// Unlock releases the lock. It is a no-op when the lock is not held.
func (l *Lock) Unlock(_ context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.locked = false
	return nil
}

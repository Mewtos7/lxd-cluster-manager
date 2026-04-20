package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Mewtos7/lx-container-weaver/internal/persistence"
)

// compile-time interface check.
var _ persistence.BootstrapLocker = (*AdvisoryLock)(nil)

// BootstrapAdvisoryLockKey is the well-known PostgreSQL advisory lock key used
// to coordinate initial cluster bootstrap across manager instances. All
// instances must use the same key to contend on the same lock.
//
// The value is derived from the FNV-1a 64-bit hash of the string
// "lx-container-weaver:bootstrap", providing a collision-resistant constant
// that is unlikely to clash with advisory locks used by other applications
// sharing the same database.
const BootstrapAdvisoryLockKey int64 = 7887792386627534949

// AdvisoryLock implements persistence.BootstrapLocker using a PostgreSQL
// session-level advisory lock (pg_try_advisory_lock / pg_advisory_unlock).
//
// The lock is held on a single dedicated connection borrowed from the pool.
// Once acquired, that connection is retained until Unlock is called, at which
// point the lock is explicitly released and the connection is returned to the
// pool. If the process exits before Unlock is called the advisory lock is
// released automatically when the underlying database session ends.
type AdvisoryLock struct {
	pool *pgxpool.Pool
	key  int64
	conn *pgxpool.Conn // non-nil while the lock is held by this instance
}

// NewAdvisoryLock returns an AdvisoryLock that contends on key. Use
// BootstrapAdvisoryLockKey as the key for the startup bootstrap lock so that
// all manager instances contend on the same lock.
func NewAdvisoryLock(pool *pgxpool.Pool, key int64) *AdvisoryLock {
	return &AdvisoryLock{pool: pool, key: key}
}

// TryLock attempts to acquire the advisory lock without blocking by calling
// pg_try_advisory_lock on a dedicated pool connection. Returns (true, nil) if
// the lock was acquired, (false, nil) if it is already held by another
// session, and (false, err) on any I/O failure.
func (l *AdvisoryLock) TryLock(ctx context.Context) (bool, error) {
	conn, err := l.pool.Acquire(ctx)
	if err != nil {
		return false, fmt.Errorf("advisory lock: acquire connection: %w", err)
	}

	var acquired bool
	if err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", l.key).Scan(&acquired); err != nil {
		conn.Release()
		return false, fmt.Errorf("advisory lock: pg_try_advisory_lock: %w", err)
	}

	if !acquired {
		conn.Release()
		return false, nil
	}

	l.conn = conn
	return true, nil
}

// Unlock releases the advisory lock and returns the dedicated connection to
// the pool. It is a no-op when the lock is not currently held by this
// instance.
func (l *AdvisoryLock) Unlock(ctx context.Context) error {
	if l.conn == nil {
		return nil
	}

	conn := l.conn
	l.conn = nil

	var released bool
	if err := conn.QueryRow(ctx, "SELECT pg_advisory_unlock($1)", l.key).Scan(&released); err != nil {
		conn.Release()
		return fmt.Errorf("advisory lock: pg_advisory_unlock: %w", err)
	}
	conn.Release()

	if !released {
		return fmt.Errorf("advisory lock: pg_advisory_unlock returned false for key %d", l.key)
	}
	return nil
}

# ADR-012 — Distributed Bootstrap Lock for Concurrent Manager Startup

| Field  | Value |
|--------|-------|
| Status | Accepted |
| Date   | 2026-04-20 |

## Context

When multiple manager instances start simultaneously (e.g. during a rolling restart or a high-availability deployment), each instance independently evaluates the bootstrap guard introduced in ADR-011. Without coordination, every instance that passes the feature-flag check and the cluster-existence check would proceed to invoke the bootstrap coordinator, potentially creating duplicate Hetzner Cloud servers or partially-initialised LXD clusters — a "split-brain bootstrap" scenario.

The cluster-existence check in ADR-011 is a read followed by a write (TOCTOU race): two instances can both read zero clusters and both proceed to provision before either has committed its result. The fix requires serialising the check-and-provision sequence across all competing instances at the database level.

## Decision

Introduce a **PostgreSQL session-level advisory lock** (`pg_try_advisory_lock` / `pg_advisory_unlock`) as the distributed coordination primitive for startup bootstrap.

### `persistence.BootstrapLocker` interface

A new interface is added to `internal/persistence/repository.go`:

```go
type BootstrapLocker interface {
    TryLock(ctx context.Context) (bool, error)
    Unlock(ctx context.Context) error
}
```

`TryLock` is non-blocking: it returns `(true, nil)` on success and `(false, nil)` when another session already holds the lock. This keeps startup fast — no manager blocks waiting.

### `postgres.AdvisoryLock`

The PostgreSQL implementation in `internal/persistence/postgres/lock.go`:

- Borrows a **dedicated connection** from the pool (`pool.Acquire`) and retains it until `Unlock` is called.  Session-level advisory locks are connection-scoped; holding the connection guarantees the lock persists for the duration of bootstrap.
- Uses the single well-known constant `BootstrapAdvisoryLockKey = 7887792386627534949` so that all manager instances contend on the same lock.
- Calls `pg_advisory_unlock` explicitly before releasing the connection to ensure prompt unlocking even when connection pooling is in use. If the process exits without calling `Unlock`, the database session ending releases the lock automatically (Scenario 2 from the issue: crash recovery).

### `memory.Lock`

A mutex-backed in-memory implementation is provided in `internal/persistence/memory/lock.go` for unit tests. It satisfies `persistence.BootstrapLocker` and is safe for concurrent use.

### Updated `Guard.Run` sequence

The lock is acquired **before** the cluster-existence check so that the check and the subsequent provisioning are atomic with respect to other competing instances:

```
INITIAL_BOOTSTRAP_ENABLED == false? ──► log "bootstrap disabled" ──► skip
│ (true)
▼
TryLock() (if locker configured)
│
├── false ──► log "another instance is handling it" ──► skip
│
└── true ──► (lock held)
      │
      ▼
      ListClusters()
      │
      ├── len > 0 ──► log "cluster already exists" ──► skip ──► Unlock
      │
      └── len == 0 ──► RunBootstrap() ──► Unlock
```

Moving the cluster check under the lock eliminates the TOCTOU race. Even if a second instance acquires the lock after the first has released it, the cluster check will find the cluster created by the first instance and skip.

### `bootstrap.WithLocker` option

`Guard` gains a `GuardOption` type and a `WithLocker(l persistence.BootstrapLocker) GuardOption` function. The locker is optional; omitting it restores the pre-ADR-011 behaviour (no distributed coordination), maintaining backward compatibility.

### Wiring in `cmd/manager/main.go`

```go
bootstrapLock := postgres.NewAdvisoryLock(pool, postgres.BootstrapAdvisoryLockKey)
bootstrapGuard := bootstrap.NewGuard(clusterRepo, logger, bootstrap.WithLocker(bootstrapLock))
```

## Rationale

- **Advisory locks over application-level locks**: advisory locks are a first-class PostgreSQL primitive. They require no schema changes, survive client crashes (auto-released on session end), and add no contention to normal query processing.
- **Session-level over transaction-level**: `pg_try_advisory_lock` (session-level) is held for the duration of the bootstrap operation regardless of whether individual sub-operations commit or roll back. This is the right scope for a startup coordinator.
- **Non-blocking `TryLock`**: blocking the startup of a competing instance would increase time-to-healthy and complicate timeout handling. Skipping immediately and logging is the correct operator experience.
- **Lock acquired before cluster check**: prevents the TOCTOU race where two instances both observe zero clusters and both provision.
- **Explicit `Unlock` + automatic session release**: belt-and-suspenders. Normal operation releases the lock cleanly; crash recovery relies on the database session ending.

## Alternatives Considered

- **Application-level mutex via a dedicated `locks` table**: achieves the same goal but requires a schema migration, manual TTL management, and cleanup on crash (harder than advisory lock auto-release).
- **Kubernetes leader election**: appropriate for long-running leader scenarios but introduces a Kubernetes dependency that the manager does not otherwise have.
- **Blocking `pg_advisory_lock`**: would serialise competing managers but could stall startup indefinitely if the lock holder hangs. Non-blocking `TryLock` + skip is safer.

## Consequences

- `postgres.AdvisoryLock` requires a `*pgxpool.Pool` reference; it is not usable with a plain `pgx.Tx`.
- The manager must have an active database connection before the bootstrap guard runs. This is already required (ADR-004).
- Unit tests use `memory.Lock` instead of a real PostgreSQL advisory lock. Integration tests against a real database would exercise the full path.
- If `INITIAL_BOOTSTRAP_ENABLED=false` (the default), the locker is never contacted. Startup remains zero-I/O for the bootstrap path.

## Related ADRs

- ADR-004 — Database Technology Choice
- ADR-005 — Hyperscaler Integration Approach
- ADR-011 — Initial Cluster Bootstrap Feature Flag and Guard

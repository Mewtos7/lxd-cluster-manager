# ADR-004 — Database Technology Choice

| Field  | Value |
|--------|-------|
| Status | Accepted |
| Date   | 2026-04-17 |

## Context

The manager needs to persist cluster state, node metadata, container/VM records, scheduling decisions, and provisioning history. The persistence layer must be reliable, easy to operate in a single-service deployment, and capable of supporting relational queries across entities (clusters → nodes → containers).

## Decision

Use **PostgreSQL** as the primary database.

## Rationale

- **Relational model fits the domain well:** Clusters, nodes, and containers have clear foreign-key relationships and benefit from JOIN queries and referential integrity.
- **Proven reliability:** PostgreSQL is production-grade with strong ACID guarantees, suitable for state that must survive process restarts and partial failures.
- **Rich ecosystem:** Mature drivers exist for all major languages (notably the Go `pgx` driver), and tooling for migrations (e.g. `golang-migrate`, `Flyway`) is well-established.
- **Operational familiarity:** PostgreSQL is widely understood and can be run as a managed service on Hetzner (or self-hosted) with minimal configuration.
- **Schema evolution:** Structured migrations are straightforward compared to document stores, which matters as the data model evolves.

## Alternatives Considered

- **SQLite:** Suitable for single-binary deployments, but poor support for concurrent writes and no network access mode makes it unsuitable for a service that may eventually scale horizontally or be accessed by multiple replicas.
- **etcd:** Excellent for distributed consensus and leader election, but not designed for relational queries. Better suited as a coordination layer (could be added alongside PostgreSQL if needed) than a primary store.
- **CockroachDB / distributed SQL:** Adds operational complexity without a clear current requirement for geographic distribution or extreme scale.

## Consequences

- PostgreSQL must be available as a dependency for running the manager (locally via Docker Compose, or as a managed instance in production).
- Database schema changes are managed through versioned migration files under `db/migrations/`.
- Connection pooling (e.g. PgBouncer or library-level pooling) should be configured for production deployments.
- The data access layer is abstracted behind an interface to allow swapping the underlying database in tests or future decisions without rewriting business logic.

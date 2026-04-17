# LX Container Weaver

Smart cluster orchestration for LXD environments with automatic scaling, consolidation, and workload management to keep infrastructure efficient and cost-aware.

[![License](https://img.shields.io/github/license/Mewtos7/lx-container-weaver?style=for-the-badge)](LICENSE)
[![Contributions Welcome](https://img.shields.io/badge/contributions-welcome-brightgreen?style=for-the-badge)](CONTRIBUTING.md)
[![Last Commit](https://img.shields.io/github/last-commit/Mewtos7/lx-container-weaver?style=for-the-badge)](https://github.com/Mewtos7/lx-container-weaver/commits/main)
[![Open Issues](https://img.shields.io/github/issues/Mewtos7/lx-container-weaver?style=for-the-badge)](https://github.com/Mewtos7/lx-container-weaver/issues)
[![Repo Size](https://img.shields.io/github/repo-size/Mewtos7/lx-container-weaver?style=for-the-badge)](https://github.com/Mewtos7/lx-container-weaver)
[![Stars](https://img.shields.io/github/stars/Mewtos7/lx-container-weaver?style=for-the-badge)](https://github.com/Mewtos7/lx-container-weaver/stargazers)
[![Forks](https://img.shields.io/github/forks/Mewtos7/lx-container-weaver?style=for-the-badge)](https://github.com/Mewtos7/lx-container-weaver/network/members)
[![Commit Activity](https://img.shields.io/github/commit-activity/m/Mewtos7/lx-container-weaver?style=for-the-badge)](https://github.com/Mewtos7/lx-container-weaver/commits/main)

## Overview

LX Container Weaver helps operate LXD clusters on cloud infrastructure by making scaling and placement decisions based on current usage and workload demand.

The goal is simple:

- Keep enough capacity for container and VM scheduling.
- Avoid overprovisioning by consolidating workloads when possible.
- Reduce cloud spend by removing underused nodes and servers.

## Core Capabilities

- Cluster bootstrap with an initial two-node LXD setup.
- Resource-aware monitoring across nodes in a cluster.
- Horizontal scale-out when new workload placement requires additional capacity.
- Workload consolidation with automatic node/server scale-in when usage drops.
- API-driven management for core entities and operations.
- Multi-cluster management support.
- Live migration support for containers and VMs within the same cluster.
- CRUD-based configuration for nodes and containers through the manager API.

## Why Use It

- Better infrastructure utilization across LXD nodes.
- Lower operational overhead through automation.
- Cost-efficient cluster behavior with continuous right-sizing.
- Flexible management model for VM and container workloads.

## Project Links

- Contributor guide: [CONTRIBUTING.md](CONTRIBUTING.md)
- License (Apache 2.0): [LICENSE](LICENSE)
- Repository: [Mewtos7/lx-container-weaver](https://github.com/Mewtos7/lx-container-weaver)

## Database Migrations

Schema changes are managed with versioned SQL migration files stored under `db/migrations/`. A lightweight Go migration runner (in `internal/migration/`) applies them in order and tracks the applied state in a `schema_migrations` table.

### Prerequisites

- [Docker](https://docs.docker.com/get-docker/) and [Docker Compose](https://docs.docker.com/compose/) for the local PostgreSQL instance.
- Go 1.25+ to build the `migrate` binary.

### Quickstart — apply migrations on a fresh database

```sh
# 1. Start a local PostgreSQL container
make db-up

# 2. Apply all pending migrations
make migrate-up
```

The first run creates the full schema (clusters, nodes, instances, placements, scaling actions, migrations, and audit events tables) as defined in `db/migrations/0001_initial_schema.up.sql`.

### Check migration status

```sh
make migrate-status
```

This prints every applied migration version and the timestamp at which it was applied.

### Roll back the latest migration

```sh
make migrate-down
```

Only the most recently applied migration is rolled back. Run the command multiple times to step further back.

### Tear down the local database

```sh
# Stops the container and removes the volume (all data is lost)
make db-down
```

### Custom database URL

All migration targets read the `DATABASE_URL` environment variable. You can override the default local value:

```sh
DATABASE_URL=postgres://user:pass@myhost:5432/mydb?sslmode=require make migrate-up
```

### Migration file naming convention

```
db/migrations/<version>_<description>.up.sql   — forward migration
db/migrations/<version>_<description>.down.sql — rollback
```

Where `<version>` is a zero-padded four-digit integer (e.g. `0001`, `0002`). Files are applied in ascending lexicographic order. Each migration runs inside a single database transaction; a failure causes the transaction to roll back and the error is surfaced immediately.

## Status

This project is under active development and open to improvements, fixes, and feature contributions.

If you want to contribute, start with [CONTRIBUTING.md](CONTRIBUTING.md).
# LX Container Weaver

Smart cluster orchestration for LXD environments with automatic scaling, consolidation, and workload management to keep infrastructure efficient and cost-aware.

[![CI](https://github.com/Mewtos7/lx-container-weaver/actions/workflows/ci.yml/badge.svg)](https://github.com/Mewtos7/lx-container-weaver/actions/workflows/ci.yml)
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

## Prerequisites

| Tool | Minimum version | Purpose |
|------|-----------------|---------|
| [Go](https://go.dev/dl/) | 1.25 | Building and running the manager |
| [Docker](https://docs.docker.com/get-docker/) | 24 | Running the local PostgreSQL container |
| [Docker Compose](https://docs.docker.com/compose/) | v2 | Orchestrating local services |

Verify your installations before continuing:

```sh
go version          # go1.25.x or later
docker --version    # Docker version 24.x or later
docker compose version  # Docker Compose version v2.x or later
```

## Quick Start

```sh
cp .env.example .env          # copy config template
make gen-api-key              # generate an API key; paste the bcrypt hash into .env
make dev-up                   # start PostgreSQL and apply migrations
# copy printed DATABASE_URL into .env (for local default: postgres://weaver:secret@localhost:5432/weaver?sslmode=disable)
make run-manager              # export .env variables and start manager
```

Run `make help` to list all available targets.

## First-Time Setup Details

If you do not want to use the quick-start flow, follow the detailed step-by-step setup below.

1. Copy the environment template:

```sh
cp .env.example .env
```

2. Generate an API key:

```sh
make gen-api-key
```

Copy the `Bcrypt` value into `.env` as `API_KEYS`, wrapped in single quotes so `$` stays literal.

3. Start the local environment:

```sh
make dev-up
```

This starts PostgreSQL and applies migrations. Copy the printed `DATABASE_URL` value into `.env`.

4. Start the manager:

```sh
make run-manager
```

5. Verify health:

```sh
curl http://localhost:8080/v1/health
# -> {"status":"ok"}
```

## Environment Variables

All variables are documented in `.env.example`. Do not commit `.env`.

| Variable | Default | Description |
|----------|---------|-------------|
| `HTTP_ADDR` | `:8080` | Address the HTTP server listens on |
| `DATABASE_URL` | `postgres://weaver:secret@localhost:5432/weaver?sslmode=disable` | PostgreSQL connection string |
| `API_KEYS` | _(empty — must be set)_ | Comma-separated bcrypt hashes of valid API keys |
| `LOG_LEVEL` | `info` | Minimum log level: `debug`, `info`, `warn`, `error` |
| `RECONCILE_INTERVAL` | `60s` | How often the orchestration loop runs |
| `SHUTDOWN_TIMEOUT` | `30s` | Maximum wait for in-flight requests on shutdown |
| `HETZNER_API_TOKEN` | _(empty — optional)_ | Hetzner Cloud API token; enables Hetzner provider |
| `INITIAL_BOOTSTRAP_ENABLED` | `false` | When `true`, the manager attempts to bootstrap the first LXD cluster at startup. Skipped automatically when any cluster already exists. |
| `BOOTSTRAP_CLUSTER_NAME` | _(empty — required when bootstrap enabled)_ | Human-readable name for the bootstrapped LXD cluster |
| `BOOTSTRAP_HETZNER_SERVER_TYPE` | _(empty — required when bootstrap enabled)_ | Hetzner Cloud server type (e.g. `cx22`) |
| `BOOTSTRAP_HETZNER_REGION` | _(empty — required when bootstrap enabled)_ | Hetzner Cloud datacenter region (e.g. `nbg1`) |
| `BOOTSTRAP_HETZNER_IMAGE` | _(empty — required when bootstrap enabled)_ | Hetzner Cloud OS image (e.g. `ubuntu-22.04`) |
| `BOOTSTRAP_TRUST_TOKEN` | _(empty — required when bootstrap enabled)_ | Shared secret authorising LXD cluster member joins |
| `BOOTSTRAP_STORAGE_DRIVER` | _(empty — required when bootstrap enabled)_ | LXD storage backend driver (e.g. `dir`, `zfs`) |
| `BOOTSTRAP_STORAGE_POOL` | _(empty — required when bootstrap enabled)_ | LXD storage pool name (e.g. `default`) |
| `BOOTSTRAP_SEED_NODE_NAME` | _(empty — required when bootstrap enabled)_ | LXD member name for the seed node (e.g. `lxd1`) |
| `BOOTSTRAP_SEED_NODE_ADDRESS` | _(empty — required when bootstrap enabled)_ | Seed node listen address `host:port` (e.g. `10.0.0.1:8443`) |
| `BOOTSTRAP_JOINER_NODE_NAME` | _(empty — required when bootstrap enabled)_ | LXD member name for the joiner node (e.g. `lxd2`) |
| `BOOTSTRAP_JOINER_NODE_ADDRESS` | _(empty — required when bootstrap enabled)_ | Joiner node listen address `host:port` (e.g. `10.0.0.2:8443`) |

## Reset Workflow

```sh
make dev-reset
make dev-up
```

`dev-reset` removes local PostgreSQL state and `dev-up` recreates it.

## Hetzner Cloud Provider

The manager ships with a first-class **Hetzner Cloud provider adapter** that implements the `HyperscalerProvider` interface (ADR-005).

### Server lifecycle operations

| Operation | Transport | Description |
|-----------|-----------|-------------|
| `ProvisionServer` | Hetzner Cloud REST API | Checks for existing server by name; creates if not found |
| `DeprovisionServer` | Hetzner Cloud REST API | Deletes the server by its numeric ID |
| `GetServer` | Hetzner Cloud REST API | Returns current server state mapped to a provider-agnostic struct |
| `ListServers` | Hetzner Cloud REST API | Returns all servers visible to the configured token |

### Provisioning flow

```
ProvisionServer(ctx, spec)
      │
      ▼
 validateSpec ──► ErrInvalidSpec (missing fields)
      │
      ▼
 client.Server.GetByName ──► server exists ──► return existing ID
      │ (nil: not found)
      ▼
 client.Server.Create ──► Hetzner Cloud API
      │
      ▼
 return serverID ──► caller (orchestrator / bootstrap flow)

DeprovisionServer(ctx, serverID)
      │
      ▼
 ParseInt(serverID) ──► error (invalid format)
      │
      ▼
 client.Server.GetByID ──► nil ──► ErrServerNotFound
      │
      ▼
 client.Server.Delete ──► Hetzner Cloud API
```

### Triggering provisioning locally

To verify the provisioning workflow locally with a real Hetzner Cloud account:

1. Set `HETZNER_API_TOKEN` in `.env` to a valid Hetzner Cloud API token.
2. Run `make run-manager`.
3. The manager logs confirm that the Hetzner Cloud provider has been initialised.

> **Note:** `ProvisionServer` creates real cloud resources when `HETZNER_API_TOKEN` is set. Use a
> dedicated Hetzner project or test token to avoid unintended charges.

## Initial Cluster Bootstrap

The manager ships with a **bootstrap guard** that controls whether the very first LXD cluster is provisioned automatically at startup (ADR-011).

### How the guard works

```
Manager startup
      │
      ▼
INITIAL_BOOTSTRAP_ENABLED == false? ──► log "bootstrap disabled" ──► skip
      │ (true)
      ▼
ClusterRepository.ListClusters()
      │
      ├── len > 0 ──► log "cluster already exists" ──► skip
      │
      └── len == 0 ──► log "invoking bootstrap coordinator" ──► RunBootstrap()
```

The guard is deterministic and **side-effect free** when the feature flag is disabled. It never mutates state until all conditions are met.

### Enabling bootstrap

Set `INITIAL_BOOTSTRAP_ENABLED=true` in `.env` **only** when all of the following are true:

- No cluster has been registered in the database yet.
- You want the manager to bootstrap the first two-node LXD cluster automatically on next startup.

Once at least one cluster exists, the bootstrap path is skipped even if the flag remains `true`.

> **Note:** Leave `INITIAL_BOOTSTRAP_ENABLED=false` (the default) for all other environments to keep startup fully side-effect free.

## Provisioning Model

All server lifecycle operations use the **Hetzner Cloud REST API** directly — no external state files are involved. `ProvisionServer` is idempotent: calling it again for an existing server name returns the existing server's ID without creating a duplicate. This means accidental loss of any local files has no impact on the management plane.

## Database Migrations

Migration targets operate on `DATABASE_URL`.

| Target | Effect |
|--------|--------|
| `make migrate-up` | Apply all pending migrations |
| `make migrate-down` | Roll back the most recently applied migration |
| `make migrate-status` | List applied migrations and timestamps |

To target a different database:

```sh
DATABASE_URL=postgres://user:pass@myhost:5432/mydb?sslmode=require make migrate-up
```

Migration files live in `db/migrations/` and follow this naming convention:

```text
db/migrations/<version>_<description>.up.sql
db/migrations/<version>_<description>.down.sql
```

Where `<version>` is a zero-padded four-digit integer (for example, `0001`, `0002`).

- `.up.sql` applies the schema change.
- `.down.sql` rolls back the same change.

Migrations are executed in ascending version order, and each migration runs inside a single database transaction.

## Local Validation

Run the full validation suite before opening a pull request:

```sh
make validate
```

This runs build, vet, fmt-check, test, and lint in sequence. Each stage can also be run individually:

| Command | What it checks |
|---------|----------------|
| `make build` | Compiles all packages |
| `make test` | Runs the full test suite |
| `make vet` | Runs `go vet` static analysis |
| `make fmt-check` | Checks that all Go files are `gofmt`-formatted |
| `make lint` | Runs `golangci-lint` (requires installation, see below) |

### Installing golangci-lint

```sh
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

See the [golangci-lint installation guide](https://golangci-lint.run/welcome/install/) for alternative methods.

### CI

Every push to `main` and every pull request triggers the same validation steps automatically via GitHub Actions (`.github/workflows/ci.yml`).

## Troubleshooting

#### `docker: command not found`

Install Docker and ensure the daemon is running.

#### `go: command not found`

Install Go and ensure it is on your `PATH`.

#### Port 5432 is already in use

Stop the conflicting PostgreSQL instance or change the port mapping in `docker-compose.yml`, then update `DATABASE_URL` in `.env`.

#### Port 8080 is already in use

Set `HTTP_ADDR` in `.env` to another port, for example `HTTP_ADDR=:9090`.

#### Manager exits with `API_KEYS is required`

Generate a key with `make gen-api-key`, update `API_KEYS` in `.env`, then run `make run-manager` again.

#### Manager exits with `DATABASE_URL is required`

Set `DATABASE_URL` in `.env`, then run `make run-manager` again.

#### Migrations fail with `connection refused`

The PostgreSQL container may still be starting. Run `make db-up` and retry.

## AI Assistance

This project uses AI tooling to assist with writing content. All contributions are reviewed and approved by a human maintainer before they are merged into the repository.

## Status

This project is under active development and open to improvements, fixes, and feature contributions.

If you want to contribute, start with [CONTRIBUTING.md](CONTRIBUTING.md).
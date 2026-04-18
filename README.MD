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

## Reset Workflow

```sh
make dev-reset
make dev-up
```

`dev-reset` removes local PostgreSQL state and `dev-up` recreates it.

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
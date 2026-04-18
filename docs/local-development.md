# Local Development Guide

This guide walks you through setting up, running, and resetting the LX Container Weaver manager on your local machine.

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

If a required tool is missing, follow the installation link in the table above and re-run the checks.

## First-time setup

### 1. Copy the environment template

```sh
cp .env.example .env
```

Open `.env` in an editor. The file is pre-populated with safe defaults for local use. The only value you must fill in is `API_KEYS` (see next step).

### 2. Generate an API key

The manager refuses to start without at least one hashed API key. The `gen-api-key` tool generates a raw key and its bcrypt hash:

```sh
make gen-api-key
```

Example output:

```
Raw key  : Tz3xQ...8Qw
Bcrypt   : $2a$12$...

Add to API_KEYS in .env or export directly:
  export API_KEYS='$2a$12$...'
```

Copy the `Bcrypt` value into `.env` (wrap it in single quotes so `$` stays literal):

```
API_KEYS='$2a$12$...'
```

Keep the **raw key** — you need it to authenticate API requests. It cannot be recovered from the hash.

To add more keys (one per client), run `make gen-api-key` again and append the new hash to `API_KEYS` as a comma-separated list:

```
API_KEYS='$2a$12$firsthash,$2a$12$secondhash'
```

### 3. Start the environment

```sh
make dev-up
```

This single command starts the PostgreSQL container and applies all pending database migrations. When it finishes you will see:

```
✓ Local environment ready.
  1. Copy .env.example to .env and set API_KEYS (run 'make gen-api-key' first).
  2. Add the database connection value printed by this command to your .env file.
  3. Run: source .env && go run ./cmd/manager
  Run 'make dev-reset' to tear down all local state.
```

Use the database connection value printed by `make dev-up` when updating `.env` before starting the manager.

### 4. Start the manager

```sh
source .env && go run ./cmd/manager
```

Or build the binary first:

```sh
make build-manager
source .env && ./manager
```

### 5. Verify

```sh
curl http://localhost:8080/v1/health
# → {"status":"ok"}
```

Protected endpoints require the raw API key generated in step 2:

```sh
curl -H "Authorization: Bearer <raw-key>" http://localhost:8080/v1/...
```

A missing, malformed, or invalid key returns `401 Unauthorized`.

## Environment variables

All variables are documented in [`.env.example`](../.env.example). The defaults in that file are suitable for local development. Do not commit `.env` — it is listed in `.gitignore`.

| Variable | Default | Description |
|----------|---------|-------------|
| `HTTP_ADDR` | `:8080` | Address the HTTP server listens on |
| `DATABASE_URL` | `postgres://weaver:secret@localhost:5432/weaver?sslmode=disable` | PostgreSQL connection string |
| `API_KEYS` | _(empty — must be set)_ | Comma-separated bcrypt hashes of valid API keys |
| `LOG_LEVEL` | `info` | Minimum log level: `debug`, `info`, `warn`, `error` |
| `RECONCILE_INTERVAL` | `60s` | How often the orchestration loop runs |
| `SHUTDOWN_TIMEOUT` | `30s` | Maximum wait for in-flight requests on shutdown |

## Reset workflow

To wipe all local state and start fresh:

```sh
make dev-reset
make dev-up
```

`dev-reset` stops the PostgreSQL container and removes its data volume. `dev-up` recreates it and re-applies all migrations.

## Database migrations

Migration targets operate on the database pointed to by `DATABASE_URL`. The Makefile default matches the Docker Compose setup.

| Target | Effect |
|--------|--------|
| `make migrate-up` | Apply all pending migrations |
| `make migrate-down` | Roll back the most recently applied migration |
| `make migrate-status` | List applied migrations and their timestamps |

To target a different database, override `DATABASE_URL`:

```sh
DATABASE_URL=postgres://user:pass@myhost:5432/mydb?sslmode=require make migrate-up
```

Migration files live in `db/migrations/` and follow the naming convention:

```
db/migrations/<version>_<description>.up.sql   — forward migration
db/migrations/<version>_<description>.down.sql — rollback
```

Where `<version>` is a zero-padded four-digit integer (e.g. `0001`, `0002`). Migrations run in ascending order, each inside a single database transaction.

## Troubleshooting

### `docker: command not found`

Docker is not installed or not on your `PATH`. Install it from [docs.docker.com/get-docker](https://docs.docker.com/get-docker/) and ensure the Docker daemon is running.

### `go: command not found`

Go is not installed or not on your `PATH`. Install it from [go.dev/dl](https://go.dev/dl/) and follow the post-install instructions to update your shell's `PATH`.

### Port 5432 is already in use

Another PostgreSQL instance is running on the default port. Either stop it or change the port mapping in `docker-compose.yml` and update `DATABASE_URL` in `.env` accordingly.

### Port 8080 is already in use

Something else is bound to 8080. Set `HTTP_ADDR` in `.env` to an alternative port (e.g. `HTTP_ADDR=:9090`).

### Manager exits with `API_KEYS is required`

`API_KEYS` is empty in your environment. Run `make gen-api-key`, copy the bcrypt hash into `.env`, and re-source the file before starting the manager.

### Manager exits with `DATABASE_URL is required`

Ensure `DATABASE_URL` is set in `.env` and that you sourced the file before starting the manager:

```sh
source .env && go run ./cmd/manager
```

### Migrations fail with `connection refused`

The PostgreSQL container may still be starting. Run `make db-up` to wait for it to become ready before running migrations.

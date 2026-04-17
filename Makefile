# LX Container Weaver — developer convenience targets.
# Run `make help` to list available commands.

DATABASE_URL ?= postgres://weaver:secret@localhost:5432/weaver?sslmode=disable

.PHONY: help
help: ## Show this help message
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

# ─── Build ────────────────────────────────────────────────────────────────────

.PHONY: build
build: ## Build all binaries
	go build ./...

.PHONY: build-manager
build-manager: ## Build the manager binary
	go build -o manager ./cmd/manager

.PHONY: build-migrate
build-migrate: ## Build the migrate binary
	go build -o migrate ./cmd/migrate

# ─── Test ─────────────────────────────────────────────────────────────────────

.PHONY: test
test: ## Run all tests
	go test ./...

# ─── Database / migrations ────────────────────────────────────────────────────

.PHONY: db-up
db-up: ## Start the local PostgreSQL container
	docker compose up -d postgres
	docker compose exec postgres sh -c 'until pg_isready -U weaver -d weaver; do sleep 1; done'

.PHONY: db-down
db-down: ## Stop and remove the local PostgreSQL container and its volume
	docker compose down -v

.PHONY: migrate-up
migrate-up: build-migrate ## Apply all pending migrations (DATABASE_URL can be overridden)
	DATABASE_URL=$(DATABASE_URL) ./migrate up

.PHONY: migrate-down
migrate-down: build-migrate ## Roll back the most recently applied migration
	DATABASE_URL=$(DATABASE_URL) ./migrate down

.PHONY: migrate-status
migrate-status: build-migrate ## Show which migrations have been applied
	DATABASE_URL=$(DATABASE_URL) ./migrate status

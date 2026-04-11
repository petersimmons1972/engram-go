.PHONY: up down restart logs build test setup setup-dry-run init

## Start engram — container fetches secrets from Infisical at startup via machine identity
## Requires POSTGRES_PASSWORD to be set in .env — run 'make init' first on a fresh install.
up:
	@if ! grep -qs '^POSTGRES_PASSWORD=' .env 2>/dev/null && [ -z "$$POSTGRES_PASSWORD" ]; then \
	    echo "ERROR: POSTGRES_PASSWORD is not set. Run 'make init' to generate a strong password."; \
	    exit 1; \
	fi
	docker compose up -d engram-go

## Stop engram (preserves volumes — never use 'down -v')
down:
	docker compose down

## Rebuild image and restart
restart: build
	docker compose up -d --force-recreate engram-go

## Build the engram-go Docker image
build:
	docker build -t engram-go:latest -t engram-go:2.3.0 .

## Tail container logs
logs:
	docker logs -f engram-go-app

## Generate .env with a strong random POSTGRES_PASSWORD (idempotent — safe to re-run)
## Run this once before 'make up' on a fresh install.
init:
	@if grep -qs '^POSTGRES_PASSWORD=' .env 2>/dev/null; then \
	    echo "POSTGRES_PASSWORD already set in .env — skipping"; \
	else \
	    PW=$$(openssl rand -hex 32); \
	    echo "POSTGRES_PASSWORD=$$PW" >> .env; \
	    echo "✓ Generated POSTGRES_PASSWORD in .env"; \
	fi

## Configure MCP client — fetches current bearer token and writes mcpServers.engram in ~/.claude.json
## Run this after: first install, container restart (if key changed), or key rotation.
## After setup, run /mcp in Claude Code to reconnect.
setup:
	go run ./cmd/engram-setup

## Preview MCP config changes without writing ~/.claude.json
setup-dry-run:
	go run ./cmd/engram-setup --dry-run

## Run tests (requires test-postgres to be running)
test:
	docker compose --profile test up -d test-postgres
	go test -race ./...

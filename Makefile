.PHONY: up down restart logs build test setup setup-dry-run init test-explore-soak

## Start engram — requires POSTGRES_PASSWORD and ENGRAM_API_KEY in .env — run 'make init' first.
up:
	@if ! grep -qs '^POSTGRES_PASSWORD=' .env 2>/dev/null && [ -z "$$POSTGRES_PASSWORD" ]; then \
	    echo "ERROR: POSTGRES_PASSWORD is not set. Run 'make init' to generate one."; \
	    exit 1; \
	fi
	@if ! grep -qs '^ENGRAM_API_KEY=' .env 2>/dev/null && [ -z "$$ENGRAM_API_KEY" ]; then \
	    echo "ERROR: ENGRAM_API_KEY is not set. Run 'make init' to generate one."; \
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
	docker build -t engram-go:latest -t engram-go:3.0.0 .

## Tail container logs
logs:
	docker logs -f engram-go-app

## Generate .env with strong random credentials (idempotent — safe to re-run).
## Run this once before 'make up' on a fresh install.
init:
	@if grep -qs '^POSTGRES_PASSWORD=' .env 2>/dev/null; then \
	    echo "POSTGRES_PASSWORD already set in .env — skipping"; \
	else \
	    PW=$$(openssl rand -hex 32); \
	    echo "POSTGRES_PASSWORD=$$PW" >> .env; \
	    echo "✓ Generated POSTGRES_PASSWORD in .env"; \
	fi
	@if grep -qs '^ENGRAM_API_KEY=' .env 2>/dev/null; then \
	    echo "ENGRAM_API_KEY already set in .env — skipping"; \
	else \
	    KEY=$$(openssl rand -hex 32); \
	    echo "ENGRAM_API_KEY=$$KEY" >> .env; \
	    echo "✓ Generated ENGRAM_API_KEY in .env"; \
	fi
	@if [ ! -f .env.machine-identity ]; then \
	    touch .env.machine-identity; \
	    echo "✓ Created .env.machine-identity (empty — add Infisical credentials here if using secret management)"; \
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

## Run the explore-context soak test (50 synthetic questions, p95 iters ≤4, p95 tokens ≤15K)
test-explore-soak:
	go test ./bench/... -v -run TestExploreContext -timeout 60s

.PHONY: eval
## Run retrieval evaluation harness
eval:
	go run ./cmd/eval/main.go $(EVAL_ARGS)

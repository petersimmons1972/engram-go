.DEFAULT_GOAL := help

BUILD_VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

.PHONY: help up down down-safe restart logs build build-postgres go-build test setup setup-dry-run init check-env test-explore-soak status install-skills install-instinct

## Show available make targets
help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'


## Ensure the external Docker networks required by docker-compose.yml exist.
## Idempotent: networks are created with `external: true` semantics matching the
## compose declaration (so existing user networks are preserved). #660 fix.
ensure-networks:
	@docker network inspect litellm_default >/dev/null 2>&1 || { \
		echo "creating litellm_default network..."; \
		docker network create litellm_default; \
	}
	@docker network inspect ai-network >/dev/null 2>&1 || { \
		echo "creating ai-network network..."; \
		docker network create ai-network; \
	}
	@echo "✓ external networks ready"

## Start engram — requires POSTGRES_PASSWORD and ENGRAM_API_KEY in .env — run 'make init' first.
up: check-env ensure-networks
	@if ! grep -qs '^POSTGRES_PASSWORD=' .env 2>/dev/null && [ -z "$$POSTGRES_PASSWORD" ]; then \
	    echo "ERROR: POSTGRES_PASSWORD is not set. Run 'make init' to generate one."; \
	    exit 1; \
	fi
	@if ! grep -qs '^ENGRAM_API_KEY=' .env 2>/dev/null && [ -z "$$ENGRAM_API_KEY" ]; then \
	    echo "ERROR: ENGRAM_API_KEY is not set. Run 'make init' to generate one."; \
	    exit 1; \
	fi
	@docker image inspect engram-postgres-cg:latest > /dev/null 2>&1 || $(MAKE) build-postgres
	docker compose up -d engram-go

## Stop engram (preserves volumes — never use 'down -v')
down:
	docker compose down

## Stop engram safely — preserves volumes (alias for 'down', explicit name for safety)
down-safe: down

## Build the custom Postgres+pgvector image (required once before 'make up' on a fresh clone)
build-postgres:
	docker build -f Dockerfile.postgres -t engram-postgres-cg:latest .

## Rebuild image and restart
restart: build
	docker compose up -d --force-recreate engram-go

## Build the engram-go Docker image
build:
	docker build -t engram-go:latest -t engram-go:3.0.0 .

## Build Go binaries with version injection
go-build:
	go build -trimpath -ldflags "-s -w -X main.Version=$(BUILD_VERSION)" -o engram ./cmd/engram
	go build -trimpath -ldflags "-s -w -X main.Version=$(BUILD_VERSION)" -o engram-setup ./cmd/engram-setup
	go build -trimpath -ldflags "-s -w -X main.Version=$(BUILD_VERSION)" -o engram-eval ./cmd/eval
	go build -trimpath -ldflags "-s -w -X main.Version=$(BUILD_VERSION)" -o instinct-benchmark ./cmd/benchmark
	go build -trimpath -ldflags "-s -w -X main.Version=$(BUILD_VERSION)" -o instinct ./cmd/instinct
	go build -trimpath -ldflags "-s -w -X main.Version=$(BUILD_VERSION)" -o longmemeval ./cmd/longmemeval
	go build -trimpath -ldflags "-s -w -X main.Version=$(BUILD_VERSION)" -o starter ./cmd/starter

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
	    BACKUP=$$HOME/.config/engram/api_key; \
	    if [ -f "$$BACKUP" ] && [ -s "$$BACKUP" ]; then \
	        KEY=$$(cat "$$BACKUP" | tr -d '[:space:]'); \
	        echo "ENGRAM_API_KEY=$$KEY" >> .env; \
	        echo "✓ Restored ENGRAM_API_KEY from $$BACKUP (key unchanged)"; \
	    else \
	        KEY=$$(openssl rand -hex 32); \
	        echo "ENGRAM_API_KEY=$$KEY" >> .env; \
	        mkdir -p "$$HOME/.config/engram" && echo "$$KEY" > "$$BACKUP" && chmod 0600 "$$BACKUP"; \
	        echo "✓ Generated ENGRAM_API_KEY in .env and backed up to $$BACKUP"; \
	    fi; \
	fi
	@if [ ! -f .env.machine-identity ]; then \
	    touch .env.machine-identity; \
	    echo "✓ Created .env.machine-identity (empty — add Infisical credentials here if using secret management)"; \
	fi
	@if ! docker volume inspect engram_pgdata >/dev/null 2>&1; then \
	    docker volume create engram_pgdata; \
	    echo "✓ Created Docker volume engram_pgdata"; \
	fi
	@# #698: the Compose file declares `ollama_storage` as an alias for the
	@# external volume `ollama_ollama_storage`. Probe + create the external name
	@# so what `docker volume ls` shows matches what the docs say.
	@if ! docker volume inspect ollama_ollama_storage >/dev/null 2>&1; then \
	    docker volume create ollama_ollama_storage; \
	    echo "✓ Created Docker volume ollama_ollama_storage"; \
	fi

## Verify .env contains no placeholder values before deploying.
check-env:
	@if grep -qsE '^(POSTGRES_PASSWORD|ENGRAM_API_KEY)=change_me' .env 2>/dev/null; then \
	    echo "ERROR: .env still contains placeholder credentials. Run 'make init'."; exit 1; \
	fi
	@if [ ! -f .env ] || ! grep -qsE '^POSTGRES_PASSWORD=.+' .env || ! grep -qsE '^ENGRAM_API_KEY=.+' .env; then \
	    echo "ERROR: .env missing or has empty credentials. Run 'make init'."; exit 1; \
	fi
	@# #701: route diagnostic to stderr so `make up > /dev/null` is clean
	@echo "✓ .env credentials look set" >&2

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

## Show live health of every stack layer (postgres, engram-go, olla, reembed workers, precision host)
## Pass NO_REMOTE=1 to skip SSH probes: make status NO_REMOTE=1
status:
	@bash infra/status.sh $(if $(NO_REMOTE),--no-remote,)

## Build and install Phase 1 instinct binaries to ~/bin
install-instinct:
	go build -ldflags "-X main.Version=$(BUILD_VERSION)" -o $(HOME)/bin/instinct-consolidate ./cmd/instinct
	go build -ldflags "-X main.Version=$(BUILD_VERSION)" -o $(HOME)/bin/instinct-audit-go ./cmd/audit
	go build -ldflags "-X main.Version=$(BUILD_VERSION)" -o $(HOME)/bin/instinct-migrate-confidence ./cmd/instinct-migrate-confidence
	@echo "✓ Installed: instinct-consolidate, instinct-audit-go, instinct-migrate-confidence"

## Install bundled Claude Code skills to ~/.claude/skills/
install-skills:
	@mkdir -p ~/.claude/skills
	@cp -r skills/* ~/.claude/skills/
	@echo "Installed engram skills to ~/.claude/skills/"

.PHONY: eval
## Run retrieval evaluation harness
eval:
	go run ./cmd/eval/main.go $(EVAL_ARGS)

## Force a Postgres backup now (A-2 / #658).
backup-now:
	docker compose exec postgres-backup sh -c 'ts=$$(date +%Y%m%d-%H%M%S); out=/backups/engram-manual-$$ts.dump; pg_dump -h postgres -U engram -Fc engram > "$$out" && echo "✓ wrote $$out ($$(du -h $$out | cut -f1))"'

## Restore-drill the most recent backup into a throwaway DB.
backup-restore-drill:
	@latest=$$(ls -t backups/engram-*.dump 2>/dev/null | head -1); \
	if [ -z "$$latest" ]; then echo "no backups found in ./backups/"; exit 1; fi; \
	echo "restoring $$latest into engram_restore_test ..."; \
	docker compose exec postgres createdb -U engram engram_restore_test 2>/dev/null || true; \
	docker compose exec -T postgres pg_restore -U engram -d engram_restore_test --clean --if-exists < $$latest; \
	docker compose exec postgres psql -U engram -d engram_restore_test -c "SELECT count(*) AS restored_memories FROM memories;"; \
	docker compose exec postgres dropdb -U engram engram_restore_test; \
	echo "✓ restore drill passed"

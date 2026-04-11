.PHONY: up down restart logs build test setup setup-dry-run

## Start engram — container fetches secrets from Infisical at startup via machine identity
up:
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

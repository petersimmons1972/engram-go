INFISICAL_DOMAIN  := https://infisical.petersimmons.com
INFISICAL_PROJECT := f49c5b01-4bd1-4883-afbd-51c1fef53a2f
INFISICAL_ENV     := prod
INFISICAL_PATH    := /engram

INFISICAL := infisical run \
	--domain $(INFISICAL_DOMAIN) \
	--projectId $(INFISICAL_PROJECT) \
	--env $(INFISICAL_ENV) \
	--path $(INFISICAL_PATH) \
	--

.PHONY: up down restart logs build test

## Start engram (secrets injected from Infisical — no .env on disk)
up:
	$(INFISICAL) docker compose up -d engram-go

## Stop engram (preserves volumes)
down:
	docker compose down

## Rebuild image and restart
restart: build
	$(INFISICAL) docker compose up -d --force-recreate engram-go

## Build the engram-go Docker image
build:
	docker build -t engram-go:latest -t engram-go:2.1.0 .

## Tail container logs
logs:
	docker logs -f engram-go-app

## Run tests (requires test-postgres to be running)
test:
	docker compose --profile test up -d test-postgres
	go test -race ./...

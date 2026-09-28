SHELL := /bin/bash
.DEFAULT_GOAL := help

# Host ports stay off the usual defaults so Pressroom can run next to other
# projects that already use 5432 and 3000.
PRESSROOM_DB_PORT ?= 5433
WEB_PORT ?= 3300
DATABASE_URL ?= postgres://pressroom:pressroom@localhost:$(PRESSROOM_DB_PORT)/pressroom?sslmode=disable
TEST_DATABASE_URL ?= postgres://pressroom:pressroom@localhost:$(PRESSROOM_DB_PORT)/pressroom_test?sslmode=disable
export PRESSROOM_DB_PORT WEB_PORT DATABASE_URL

.PHONY: help
help: ## List targets
	@grep -E '^[a-z-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

## ---- run

.PHONY: up down db
up: ## Run the whole stack in Docker (http://localhost:3300)
	docker compose up --build

down: ## Stop the stack and delete its data
	docker compose down -v

db: ## Start only Postgres
	docker compose up -d postgres

.PHONY: migrate seed dev-api dev-worker dev-web
migrate: ## Apply migrations to DATABASE_URL
	go run ./cmd/pressroomctl migrate

seed: migrate ## Load the demo crew, orders and history
	go run ./cmd/pressroomctl seed

dev-api: ## Run the API on :8080
	LOG_FORMAT=text go run ./cmd/api

dev-worker: ## Run the worker on :8081
	LOG_FORMAT=text PORT=8081 go run ./cmd/worker

dev-web: ## Run the dashboard on :5173
	cd web && pnpm install && pnpm dev

## ---- quality

.PHONY: generate check-generated fmt lint test test-integration test-web ci
generate: ## Regenerate GraphQL server and client code
	cd internal/adapter/graphql && go tool gqlgen generate --config gqlgen.yml
	cd web && pnpm codegen

check-generated: generate ## Fail if generated code is stale
	git diff --exit-code -- internal/adapter/graphql web/src/gql

fmt: ## Format Go code
	gofmt -w cmd internal

lint: ## Vet and check formatting
	go vet ./...
	@test -z "$$(gofmt -l cmd internal)" || (gofmt -l cmd internal; echo "run make fmt"; exit 1)

test: ## Unit tests (no database needed)
	go test -race ./...

test-integration: ## Unit and integration tests against Postgres
	docker compose up -d postgres
	docker compose exec -T postgres sh -c 'psql -U pressroom -tc "SELECT 1 FROM pg_database WHERE datname = '"'"'pressroom_test'"'"'" | grep -q 1 || createdb -U pressroom pressroom_test'
	PRESSROOM_TEST_DATABASE_URL=$(TEST_DATABASE_URL) go test -race -count=1 ./...

test-web: ## Dashboard typecheck and unit tests
	cd web && pnpm typecheck && pnpm test

ci: lint test-integration test-web ## Everything CI runs

## ---- build

.PHONY: build images
build: ## Build the three binaries into ./bin
	CGO_ENABLED=0 go build -trimpath -o bin/ ./cmd/...

images: ## Build the container images
	docker build --target api -t pressroom-api .
	docker build --target worker -t pressroom-worker .
	docker build --target ctl -t pressroom-ctl .
	docker build -t pressroom-web web

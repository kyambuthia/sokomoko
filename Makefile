SHELL := /bin/bash
GO ?= go
COMPOSE ?= docker compose

DATABASE_URL ?= postgres://sokomoko:sokomoko@localhost:5432/sokomoko?sslmode=disable
SOKOMOKO_TEST_DATABASE_URL ?= postgres://sokomoko:sokomoko@localhost:5432/sokomoko_test?sslmode=disable
export DATABASE_URL SOKOMOKO_TEST_DATABASE_URL

.PHONY: help db-up db-down db-test-create migrate seed run build test test-race lint fmt docker

help: ## List targets
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-16s %s\n", $$1, $$2}'

db-up: ## Start the local PostgreSQL container
	$(COMPOSE) up -d db

db-down: ## Stop local containers
	$(COMPOSE) down

db-test-create: ## Create the test database inside the compose PostgreSQL
	$(COMPOSE) exec -T db psql -U sokomoko -d sokomoko -tc "SELECT 1 FROM pg_database WHERE datname = 'sokomoko_test'" | grep -q 1 || \
		$(COMPOSE) exec -T db createdb -U sokomoko sokomoko_test

migrate: ## Apply database migrations
	$(GO) run ./cmd/sokomoko migrate

seed: ## Apply migrations and seed demo data
	$(GO) run ./cmd/sokomoko seed

run: ## Run the server with demo data
	$(GO) run ./cmd/sokomoko serve --seed

build: ## Build the binary into bin/
	CGO_ENABLED=0 $(GO) build -trimpath -o bin/sokomoko ./cmd/sokomoko

test: ## Run all tests (requires PostgreSQL)
	SOKOMOKO_REQUIRE_DB_TESTS=1 $(GO) test ./...

test-race: ## Run all tests with the race detector
	SOKOMOKO_REQUIRE_DB_TESTS=1 $(GO) test -race ./...

lint: ## gofmt check, go vet, and JavaScript syntax check
	@unformatted="$$(gofmt -l $$(git ls-files '*.go'))"; if [ -n "$$unformatted" ]; then echo "$$unformatted"; exit 1; fi
	$(GO) vet ./...
	@for f in internal/ui/static/scripts/*.js; do node --check "$$f" || exit 1; done

fmt: ## Format Go code
	gofmt -w $$(git ls-files '*.go')

docker: ## Build the production image
	docker build -t sokomoko:local .

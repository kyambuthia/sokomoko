SHELL := /bin/bash
GO ?= go
COMPOSE ?= docker compose

DATABASE_URL ?= postgres://sokomoko:sokomoko@localhost:5432/sokomoko?sslmode=disable
SOKOMOKO_TEST_DATABASE_URL ?= postgres://sokomoko:sokomoko@localhost:5432/sokomoko_test?sslmode=disable
export DATABASE_URL SOKOMOKO_TEST_DATABASE_URL

.PHONY: help db-up db-down db-test-create migrate seed run build test test-race test-isolated test-ui lint fmt docker vuln smoke-image

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

run: ## Run the server with demo data (loopback by default)
	BIND_HOST="$(if $(BIND_HOST),$(BIND_HOST),127.0.0.1)" $(GO) run ./cmd/sokomoko serve --seed

build: ## Build the binary into bin/
	CGO_ENABLED=0 $(GO) build -trimpath -o bin/sokomoko ./cmd/sokomoko

test: ## Run all tests (requires PostgreSQL)
	SOKOMOKO_REQUIRE_DB_TESTS=1 $(GO) test ./...

test-race: ## Run all tests with the race detector
	SOKOMOKO_REQUIRE_DB_TESTS=1 $(GO) test -race ./...

test-isolated: ## Run race tests with a disposable loopback PostgreSQL container
	GO="$(GO)" bash scripts/test-isolated.sh

test-ui: ## Run JavaScript component behavior tests (Node.js 22+)
	node --test tests/ui/*.test.mjs

lint: ## Go formatting and analysis, JavaScript and shell syntax checks
	bash scripts/check-shell.sh
	bash scripts/go-format.sh --check
	$(GO) vet ./...
	$(GO) run honnef.co/go/tools/cmd/staticcheck@2026.2.2 ./...
	@for f in internal/ui/static/scripts/*.js internal/ui/static/scripts/ui/*.js; do node --check "$$f" || exit 1; done

fmt: ## Format Go code
	bash scripts/go-format.sh --write

docker: ## Build the production image
	docker build -t sokomoko:local .

vuln: ## Check reachable vulnerabilities with a pinned scanner
	$(GO) run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...

smoke-image: ## Verify a local production image using isolated disposable containers
	bash scripts/smoke-image.sh sokomoko:local

.PHONY: chat-crypto chat-crypto-test chat-crypto-check chat-crypto-audit
chat-crypto: ## Rebuild pinned browser MLS assets and third-party notices
	bash scripts/build-chat-crypto.sh

chat-crypto-test: ## Verify browser MLS protocol operations
	cargo fmt --manifest-path web/crypto/Cargo.toml --check
	cargo test --manifest-path web/crypto/Cargo.toml --locked

chat-crypto-check: ## Rebuild separately and verify checked-in encryption assets match
	bash scripts/check-chat-crypto.sh

chat-crypto-audit: ## Audit all locked Rust dependencies (requires cargo-audit 0.22.2)
	bash scripts/audit-chat-crypto.sh

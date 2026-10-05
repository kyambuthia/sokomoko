# Getting Started with Sokomoko

Contributor quick start for the Go + PostgreSQL + server-rendered app.

## 1. Boot the app locally

Add the local hostnames used by the app:

```text
127.0.0.1 localhost
127.0.0.1 admin.localhost
127.0.0.1 partner.localhost
```

Start PostgreSQL and the app with demo data:

```bash
make db-up        # PostgreSQL 17 in Docker on localhost:5432
make run          # migrate, seed, and serve on :6969
```

Or run everything in containers with `docker compose up --build`.

Open:

- `http://localhost:6969` for the storefront
- `http://admin.localhost:6969` for admin and staff
- `http://partner.localhost:6969` for partner setup and fulfillment

To apply migrations only:

```bash
go run ./cmd/sokomoko migrate
```

To apply migrations and seed demo data without starting the server:

```bash
go run ./cmd/sokomoko seed
```

## 2. Know the shape of the app

The app is routed by host, not by one shared URL space:

- `localhost` serves the customer storefront, cart, checkout, and account pages.
- `admin.localhost` serves admin bootstrap, admin login, dashboards, reporting, and team tools.
- `partner.localhost` serves store setup, catalog management, and fulfillment pages.

The normal request flow is:

1. `cmd/sokomoko` loads and validates config, opens PostgreSQL, applies migrations, composes the app, and starts the HTTP server.
2. `internal/app` wires the feature services, templates, static files, and middleware.
3. `internal/routes` registers host-specific handlers.
4. `internal/service/*` holds feature rules and view-facing domain logic.
5. `internal/db` owns SQL, transactions, and persistence concerns.

More detail lives in [docs/ARCHITECTURE_OVERVIEW.md](/home/mbuthi/Projects/sokomoko/docs/ARCHITECTURE_OVERVIEW.md).

## 3. Know where to make changes

- Add or change routes in `internal/routes`.
- Add business rules in `internal/service`.
- Add or change SQL in `internal/db`; schema changes go in a new file in `internal/db/migrations`.
- Change shared wiring and middleware in `internal/app` and `cmd/sokomoko`.
- Change pages and assets in `internal/ui/templates` and `internal/ui/static`.

If a change crosses layers, start at the service boundary and keep the route and DB changes narrow.

## 4. Run the tests that matter

DB-backed tests need PostgreSQL. Each package gets its own throwaway schema:

```bash
make db-up && make db-test-create
export SOKOMOKO_TEST_DATABASE_URL=postgres://sokomoko:sokomoko@localhost:5432/sokomoko_test?sslmode=disable
make test          # or: go test ./...
make test-race
make lint
```

Common targeted runs:

```bash
go test ./internal/db -run TestConcurrentCheckoutNeverOversells
go test ./internal/auth
go test ./internal/service/checkout
go test ./cmd/sokomoko
```

## 5. Current priorities

See [docs/roadmap/ROADMAP.md](roadmap/ROADMAP.md). The launch blockers are real
payments, backups, secrets handling, a TLS proxy guide, and legal pages.

## 6. Read these next

- [README.md](../README.md)
- [docs/ARCHITECTURE_OVERVIEW.md](ARCHITECTURE_OVERVIEW.md)
- [docs/authentication.md](authentication.md)
- [docs/roadmap/ROADMAP.md](roadmap/ROADMAP.md)

# Sokomoko

A multi-vendor marketplace built with Go, PostgreSQL, and server-rendered HTML
enhanced by small custom web components.

## Stack

- Go 1.25 (`net/http`, `html/template`, `log/slog`)
- PostgreSQL 15+ via `pgx` (`database/sql` driver), versioned embedded migrations
- Server-side rendered templates and static assets embedded in the binary
- Progressive-enhancement web components (`internal/ui/static/scripts/components.js`)

## Repo layout

```text
cmd/sokomoko/              CLI: serve, migrate, seed; host routing and middleware
internal/app/              App composition, rendering, middleware, rate limiting
internal/auth/             Sessions, login, signup, password reset, CSRF, throttling
internal/config/           Environment configuration and validation
internal/db/               PostgreSQL store, queries, transactions
internal/db/migrations/    Versioned SQL migrations (embedded)
internal/db/dbtest/        Isolated per-test PostgreSQL schemas
internal/money/            Integer-cent money type
internal/routes/           HTTP handlers, JSON endpoints, page view models
internal/service/          Feature services (catalog, commerce, checkout, ...)
internal/ui/               Templates, static assets, template helpers
docs/                      Architecture, roadmap, design notes
```

## Quick start

With Docker (PostgreSQL and the app):

```bash
docker compose up --build
```

Or run PostgreSQL in Docker and the app with Go:

```bash
make db-up            # PostgreSQL on localhost:5432 (user/password: sokomoko)
make run              # migrate, seed demo data, serve on :6969
```

Add these host entries so the three surfaces resolve locally:

```text
127.0.0.1 localhost admin.localhost partner.localhost
```

| Host | Purpose |
| --- | --- |
| `http://localhost:6969` | Storefront: browse, cart, checkout, account |
| `http://admin.localhost:6969` | Platform admin: first-run `/setup`, orders, reports, team, audit |
| `http://partner.localhost:6969` | Partner workspace: store setup, products, fulfillment |

First-run admin setup happens at `admin.localhost/setup` (loopback only, or with
`ADMIN_SETUP_TOKEN`), or seed an admin with `ADMIN_PASSWORD` when running `seed`.

## Configuration

All settings are environment variables; see [`.env.example`](.env.example) for
the full list with comments. The important ones:

| Variable | Default | Notes |
| --- | --- | --- |
| `DATABASE_URL` | `postgres://sokomoko@localhost:5432/sokomoko?sslmode=disable` | Required in production |
| `ENV` | `development` | `production` enables secure cookies, JSON logs, strict validation |
| `PORT` | `6969` | |
| `ALLOWED_HOSTS` | local hosts | Required in production |
| `PASSWORD_RESET_BASE_URL` | | Required in production |
| `TRUST_PROXY_HEADERS` | `false` | Trust `X-Forwarded-For`/`-Proto`; only behind a proxy that overwrites them |
| `DB_MAX_OPEN_CONNS` / `DB_MAX_IDLE_CONNS` | `20` / `10` | Pool sizing |
| `DB_QUERY_TIMEOUT_SECONDS` | `10` | Upper bound on any single store call |
| `LOG_FORMAT` / `LOG_LEVEL` | `text` (`json` in production) / `info` | |
| `POST_RATE_LIMIT_MAX` | `0` (off) | POSTs per client IP per window |
| `AUTH_ABUSE_MAX_FAILURES` | `0` (off) | Login/reset failures before backoff |
| `SMTP_HOST`, `SMTP_FROM`, ... | | Password reset email delivery |

The server refuses to start in production when required settings are missing.

## Development

```bash
make help        # list targets
make migrate     # apply migrations
make seed        # migrations + demo partners and catalog
make test        # full test suite (needs PostgreSQL)
make test-race   # with the race detector
make lint        # gofmt, go vet, JavaScript syntax check
make docker      # build the production image
```

Tests that need a database read `SOKOMOKO_TEST_DATABASE_URL` and create a
throwaway schema per package, so they can share one database and run in
parallel. Without the variable they are skipped; CI sets
`SOKOMOKO_REQUIRE_DB_TESTS=1` so a missing database fails the build instead.

```bash
make db-up && make db-test-create
SOKOMOKO_TEST_DATABASE_URL=postgres://sokomoko:sokomoko@localhost:5432/sokomoko_test?sslmode=disable make test
```

## Database migrations

Migrations live in `internal/db/migrations/NNNN_description.sql`, are embedded in
the binary, and run in order inside transactions. A PostgreSQL advisory lock
stops two instances from migrating at once. `serve` applies pending migrations
on startup; `migrate` applies them and exits. Never edit an applied migration;
add a new one.

## Deployment

The `Dockerfile` builds a static binary into a distroless, non-root image
(about 60 MB) that defaults to `ENV=production` and port 8080. Behind a TLS
terminating proxy set `TRUST_PROXY_HEADERS=true`. Health endpoints on every host:

- `/healthz` liveness
- `/readyz` database readiness

A background job releases expired checkout stock holds every minute and prunes
expired sessions and reset tokens every five minutes.

## Docs

- [`docs/ARCHITECTURE_OVERVIEW.md`](docs/ARCHITECTURE_OVERVIEW.md): layers, data model, request flow
- [`docs/roadmap/ROADMAP.md`](docs/roadmap/ROADMAP.md): production status and feature roadmap
- [`docs/GETTING_STARTED.md`](docs/GETTING_STARTED.md): first contribution walkthrough
- [`docs/authentication.md`](docs/authentication.md): auth flows
- [`docs/design/`](docs/design/): UI design system

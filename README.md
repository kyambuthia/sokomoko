# Sokomoko

Sokomoko is a server-rendered marketplace written in Go. One binary serves
three web workspaces and a JSON API from a single PostgreSQL database:

| Host | Who uses it | What it does |
| --- | --- | --- |
| `localhost` (storefront) | Customers | Browse, search, cart, checkout, order history |
| `admin.localhost` | Admins and staff | Metrics, orders, reports, team, audit log |
| `partner.localhost` | Admins, staff, partners | Store setup, product entry, order fulfillment |
| any host, `/api/v1/...` | Browser scripts and API clients | JSON API for the same features |

## Stack

- Go 1.26, standard library `net/http` and `html/template`
- PostgreSQL 17 through `pgx` (`database/sql` driver)
- `golang.org/x/crypto/bcrypt` for passwords
- One CSS design system and ES-module web components, with no build step
  ([docs/design.md](docs/design.md)): monochrome, local JetBrains Mono,
  Tabler icons, mobile first
- Templates, static assets, and SQL migrations are embedded in the binary

## Quick start

With Docker:

```bash
docker compose up --build
```

Without Docker (needs a local PostgreSQL):

```bash
make db-up    # start PostgreSQL from docker-compose.yml, or set DATABASE_URL to your own server
make run      # migrate, seed demo data, and serve on :6969
```

The binary reads only process environment variables; it does not load `.env`
files. `.env.example` lists every setting. See
[docs/07-configuration.md](docs/07-configuration.md).

Then open:

- Storefront: <http://localhost:6969>
- Admin: <http://admin.localhost:6969> (first visit goes to `/setup`)
- Partner: <http://partner.localhost:6969>

## Commands

```text
sokomoko serve [--seed]   Start HTTP. Development auto-migrates; production only checks the schema.
sokomoko migrate          Apply pending migrations and exit.
sokomoko seed             Migrate and load demo data (refused in production).
sokomoko healthcheck      Probe /readyz locally (used by the container HEALTHCHECK).
```

Common `make` targets: `run`, `build`, `test`, `test-race`, `lint`, `fmt`,
`docker`, `smoke-image`, `vuln`. Run `make help` for the full list.

## Repository layout

```text
cmd/sokomoko/      Entry point: commands, server wiring, host router, integration tests
internal/
  app/             App container, service interfaces, middleware, rendering helpers
  routes/          HTML handlers for the storefront, admin, and partner hosts
  api/             JSON API under /api/v1
  auth/            Sessions, login, signup, setup, password reset, CSRF, abuse limits
  service/         Business logic: account, admin, catalog, checkout, commerce, partner, payment
  db/              PostgreSQL store, embedded migrations, seed data, test helpers (dbtest/)
  money/           Integer cents type
  lifecycle/       Graceful shutdown, readiness draining, background maintenance
  bootstrap/       Migrate and seed orchestration
  config/          Environment configuration and validation
  notify/          SMTP sender for password-reset email
  ui/              Embedded templates, static assets, template functions
deploy/            Production Compose file, Caddy reverse proxy, env example
scripts/           Image smoke test
docs/              Design and operations documentation
```

## Documentation

Start with [docs/README.md](docs/README.md). It indexes the overview,
architecture, HTTP surface, security, data model, frontend, configuration,
development, deployment, and known limitations.

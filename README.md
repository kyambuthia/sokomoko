# Sokomoko

Sokomoko is a single-process e-commerce application written in Go. It serves
server-rendered HTML, stores application state in SQLite, and exposes separate
storefront, admin, and partner host surfaces.

The executable source is the authority for behavior. The most useful starting
points are:

- [server startup](cmd/sokomoko/serve.go)
- [host and middleware routing](cmd/sokomoko/server.go)
- [route registration](internal/routes/register.go)
- [service composition](internal/app/compose.go)
- [embedded schema](internal/db/schema.sql)
- [template parsing](internal/ui/templates.go)

## Documentation order

Read the maintained documentation in this order:

1. [Getting started](docs/01-getting-started.md)
2. [Architecture](docs/02-architecture.md)
3. [Routes](docs/03-routes.md)
4. [Authentication](docs/04-authentication.md)
5. [Database and commerce](docs/05-database.md)
6. [UI and assets](docs/06-ui.md)
7. [Operations and configuration](docs/07-operations.md)
8. [Known issues](docs/08-known-issues.md)
9. [Roadmap](docs/09-roadmap.md)
10. [Contributor rules](docs/10-contributing.md)

## Stack

- Go, net/http, and html/template
- SQLite through github.com/ncruces/go-sqlite3
- Embedded templates and static assets through embed.FS
- Vanilla CSS and JavaScript

## Run locally

Add the hostnames used by the host router:

    127.0.0.1 localhost
    127.0.0.1 admin.localhost
    127.0.0.1 partner.localhost

Apply the schema and start without seed data:

    go run ./cmd/sokomoko migrate
    go run ./cmd/sokomoko serve

For a local database with bootstrap catalog data:

    go run ./cmd/sokomoko serve --seed

The default server address is http://localhost:6969.

Use these host surfaces:

- http://localhost:6969 — customer storefront, cart, checkout, and account
- http://admin.localhost:6969 — admin bootstrap and admin/staff operations
- http://partner.localhost:6969 — partner-facing setup, catalog, and fulfillment views

## Configuration

The application reads environment variables in
[internal/config/config.go](internal/config/config.go).

| Variable | Default | Purpose |
| --- | --- | --- |
| ENV | development | Enables production cookie behavior when set to production |
| PORT | 6969 | HTTP listen port |
| DB_PATH | ./db/t.db | SQLite database path |
| ALLOWED_HOSTS | built-in local hosts | Comma-separated trusted hosts |
| ADMIN_SETUP_TOKEN | empty | Required for non-loopback admin setup requests |
| SESSION_COOKIE_DOMAIN | empty | Optional shared cookie domain |
| POST_RATE_LIMIT_MAX | 0 | Enables the in-memory IP limiter when greater than zero |
| POST_RATE_LIMIT_WINDOW_SECONDS | 60 | Rate-limit window |
| AUTH_ABUSE_MAX_FAILURES | 0 | Enables auth failure backoff when greater than zero |
| AUTH_ABUSE_BACKOFF_BASE_SECONDS | 1 | Initial auth backoff |
| AUTH_ABUSE_BACKOFF_MAX_SECONDS | 300 | Maximum auth backoff |
| PASSWORD_RESET_BASE_URL | empty | Optional absolute base URL for reset links |
| SMTP_HOST | empty | SMTP host for reset email |
| SMTP_PORT | 587 | SMTP port |
| SMTP_USERNAME | empty | SMTP username |
| SMTP_PASSWORD | empty | SMTP password |
| SMTP_FROM | empty | Reset-email sender address |

Admin seed data additionally reads ADMIN_USERNAME, ADMIN_EMAIL, and
ADMIN_PASSWORD from the environment. Do not use the built-in partner seed
credentials outside a disposable development database; see
[known issues](docs/08-known-issues.md).

## Development commands

    go run ./cmd/sokomoko migrate
    go run ./cmd/sokomoko seed
    go run ./cmd/sokomoko serve
    go run ./cmd/sokomoko serve --seed
    go build ./cmd/sokomoko
    go test ./...
    go vet ./internal/... ./cmd/...
    gofmt -w ./cmd ./internal

seed applies the schema and runs the seed workflows before exiting.
serve --seed does the same during server startup.

# Development

## Prerequisites

- Go 1.26.9 (from `go.mod`)
- PostgreSQL 17, either from `docker compose` or your own server
- Node.js, only for `make lint`'s JavaScript syntax check
- Docker, for the image build and smoke test

## Running locally

```bash
make db-up                 # PostgreSQL on 127.0.0.1:5432 (user/password/db: sokomoko)
make run                   # go run ./cmd/sokomoko serve --seed
```

Or run the whole stack in containers with `docker compose up --build`
(storefront on `:6969`, admin setup token `local-setup-token` by default).

The Makefile exports `DATABASE_URL` and `SOKOMOKO_TEST_DATABASE_URL` pointing at
the Compose database unless you set them yourself.

In development `serve` applies migrations automatically. Use the admin host's
`/setup` (allowed from loopback when no token is set), or set `ADMIN_PASSWORD`
before seeding to get an admin account.

`*.localhost` names resolve to 127.0.0.1 in modern browsers, so
`http://admin.localhost:6969` and `http://partner.localhost:6969` work without
editing `/etc/hosts`. `curl` needs `-H 'Host: admin.localhost'`.

When you run from the repository root, CSS, JS, and image edits under
`internal/ui/static/` show on reload. Template changes need a restart, because
templates are parsed once at startup from the embedded copy.

## Make targets

| Target | Does |
| --- | --- |
| `make help` | List targets |
| `make db-up` / `db-down` | Start or stop the Compose PostgreSQL |
| `make db-test-create` | Create `sokomoko_test` inside the Compose database |
| `make migrate` | Apply migrations |
| `make seed` | Migrate and seed demo data |
| `make run` | Serve with demo data |
| `make build` | Static binary in `bin/sokomoko` |
| `make test` | `go test ./...` with database tests required |
| `make test-race` | Same with `-race` |
| `make lint` | `gofmt` check, `go vet`, `node --check` on every script |
| `make fmt` | `gofmt -w` |
| `make vuln` | `govulncheck` (pinned v1.9.0) |
| `make docker` | Build `sokomoko:local` |
| `make smoke-image` | Run `scripts/smoke-image.sh` against `sokomoko:local` |

## Tests

```bash
make db-up && make db-test-create
make test
```

Database tests use `internal/db/dbtest`. Each test gets a fresh PostgreSQL
schema (via `search_path` in the returned DSN), and the schema is dropped when
the test ends, so packages run in parallel against one database. Without
`SOKOMOKO_TEST_DATABASE_URL` those tests are skipped; with
`SOKOMOKO_REQUIRE_DB_TESTS=1` a missing database fails the run.

Where tests live:

| Location | Kind |
| --- | --- |
| `cmd/sokomoko/*_integration_test.go` | End-to-end HTTP tests through the real router, middleware, and database: API, checkout, products, page contracts, commands |
| `cmd/sokomoko/role_routes_test.go`, `main_test.go`, `healthcheck_test.go` | Routing, role access, command parsing, probe behaviour |
| `internal/db/*_test.go` | Store behaviour against PostgreSQL, including migrations |
| `internal/service/*/..._test.go` | Service rules with fakes or a real store |
| `internal/auth/*_test.go` | Login, sessions, CSRF, hardening |
| `internal/app`, `config`, `lifecycle`, `money`, `notify`, `ui`, `routes` | Unit tests |

## CI

`.github/workflows/ci.yml` runs on every push and pull request, on manual
dispatch, and weekly:

- **Test** job, with a PostgreSQL 17 service: `gofmt` check, `go mod tidy`
  diff check, `go vet`, `govulncheck`, `node --check` for scripts, and
  `go test -race ./...` with database tests required.
- **Docker image** job: builds the image, runs `scripts/smoke-image.sh`, and
  validates the production Compose file against the example env file.

Dependabot (`.github/dependabot.yml`) opens weekly updates for Go modules, the
Dockerfile, and GitHub Actions.

## Conventions

- **Layering.** Handlers do not run SQL; services do not write HTTP responses.
  Add a store method to the service's `store` interface when a service needs it.
- **Errors.** Define sentinel errors in the service and map them to status codes
  and messages in the handler.
- **Transactions.** Use `Store.withTx`. The body may run up to three times, so
  keep side effects (email, logging that matters) outside it.
- **Money.** Use `money.Cents` everywhere. Convert to `float64` only in API DTOs.
- **Schema.** Add a new numbered migration; never edit an applied one.
- **Forms first.** New features must work as plain HTML forms. JavaScript may
  enhance them through `/api/v1`.
- **CSP.** No inline scripts or remote assets. Put scripts in
  `static/scripts/` and load them with the `asset` helper.
- **Formatting.** `gofmt` for Go. Match the existing style for CSS and JS
  (tokens, BEM classes, custom elements).

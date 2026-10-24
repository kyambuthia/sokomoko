# Development

## Prerequisites

- Go 1.26.10 (from `go.mod`)
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
| `make test-isolated` | Full race suite with disposable PostgreSQL 17; requires local Docker |
| `make test-ui` | JavaScript component behavior tests with the built-in Node.js test runner; requires Node.js 22+ |
| `make lint` | `gofmt` check (including new files), `go vet`, pinned staticcheck, JavaScript syntax and declared-interpreter shell syntax checks |
| `make fmt` | `gofmt -w` |
| `make vuln` | `govulncheck` (pinned v1.9.0) |
| `make docker` | Build `sokomoko:local` |
| `make smoke-image` | Run `scripts/smoke-image.sh` against `sokomoko:local` |
| `make chat-crypto-test` | Check Rust formatting and run the MLS protocol test |
| `make chat-crypto` | Regenerate embedded encryption assets and license notices |
| `make chat-crypto-check` | Verify generated encryption assets match source without changing them |

Shell syntax is checked by `bash scripts/check-shell.sh` in both `make lint`
and CI. It includes tracked and untracked `.sh` files that are not Git-ignored,
including deployment helpers. Bash scripts use `bash -n`; POSIX shell scripts
use `sh -n`. The check does not execute their commands or load environment files.
Unsupported interpreter declarations fail explicitly. This checks syntax rather
than runtime behavior; keep integration checks for migrations, backups and
container lifecycle behavior.

## Tests

`make test-ui` runs the JavaScript behavior tests in `tests/ui` without npm
dependencies. Search tests use controlled network responses and mock timers to
check stale responses, debounce cancellation and dismissal. Cart tests check
ordered reads and writes, recovery after failures, badge initialization and
quantity updates during removal. Quantity tests check autosubmit timer cleanup, invalid intermediate edits and
constraint checks at debounce expiry. Order progress tests exercise real rendering
and controlled polling responses, including terminal states, slow requests,
disconnection and reconnection. Payment polling tests also cover final-state
allowlisting, expiry during slow requests and delayed timer callbacks. DOM presentation is
stubbed, so these tests do not replace browser accessibility or layout checks.
The same command runs in CI.

```bash
make db-up && make db-test-create
make test
```

For an isolated run without configuring or reusing a database:

```bash
make test-isolated
# Or select packages and flags:
bash scripts/test-isolated.sh -race ./internal/db -run TestService -count=1
```

This starts `postgres:17-alpine` with a random port published only on
`127.0.0.1`, overrides both database URL variables for the test process, and
requires database tests to run. Its default suite uses `-count=1` to bypass
Go test result caching. It removes its own container and anonymous
storage after success, test failure, or a handled interrupt. No development
containers, host directories or named volumes are used. The password is a
fixed disposable fixture, not a deployment credential. Use a local Docker
engine; remote Docker port mappings are not reachable through local loopback.
An uncatchable kill or daemon outage can prevent cleanup; the container is
also configured for automatic removal when it stops.

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

## Browser cryptography

Go embeds the pinned OpenMLS JS/WASM artifacts. After changing `web/crypto`,
run `make chat-crypto-test` and `make chat-crypto` using the versions in
[encrypted messaging](encrypted-messaging.md). Review the generated assets and
license notices together with the Rust source and Cargo.lock. The build script
remaps local filesystem paths; local Rust targets are excluded from Git and
Docker build contexts. Do not enable cryptographic debug logging.

`rust-toolchain.toml` pins the crypto compiler and wasm target. CI runs the
native protocol test and `make chat-crypto-check`, which rebuilds JS/WASM and
license notices in a temporary directory and compares them with embedded
assets. The check does not overwrite the working tree. If it fails, run
`make chat-crypto`, inspect the changes, and rerun the check. Updating Rust,
wasm-bindgen or dependencies is a source-and-artifact change to review together.

Fulfilment completion follows the courier's delivered order state. Seller
retries of dispatch preserve that state; completed filters and counts include
courier deliveries even when the seller preparation state remains dispatched.

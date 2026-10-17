# Architecture

Sokomoko is a layered monolith. HTTP handlers call services, services call the
PostgreSQL store, and the store owns all SQL. Dependencies point downward only.

```text
cmd/sokomoko            commands, config, server wiring, host router
      |
internal/routes         HTML handlers (storefront, admin, partner)
internal/api            JSON handlers (/api/v1)
internal/auth           auth handlers and middleware
      |
internal/app            App container: service interfaces, middleware, Render/JSON
      |
internal/service/*      business rules, one package per domain
      |
internal/db             *db.Store: SQL, transactions, migrations
      |
PostgreSQL
```

Supporting packages with no upward dependencies: `money`, `slug`, `config`,
`lifecycle`, `bootstrap`, `notify`, `ui`.

## Packages

| Package | Responsibility |
| --- | --- |
| `cmd/sokomoko` | Parses the command (`serve`, `migrate`, `seed`, `healthcheck`), loads config, sets up `slog`, opens the store, builds the muxes and middleware chain, runs the server. |
| `internal/app` | `App` holds every dependency behind an interface (`CatalogService`, `CheckoutService`, `AuthService`, ...). `Compose` builds the concrete services from a `*db.Store`. Also holds middleware and the `Render`/`RenderStatus`/`JSON` helpers. |
| `internal/routes` | HTML handlers. They parse forms, call one service, map service errors to messages, and render a template. Uses a one-shot flash cookie for post-redirect messages. |
| `internal/api` | The versioned JSON API. Same services as `routes`, with DTOs that convert `money.Cents` to JSON numbers. |
| `internal/auth` | Login, signup, staff signup, first-run setup, password reset, logout, session lookup, role middleware, CSRF middleware, in-memory abuse limiter. |
| `internal/service/catalog` | Product listing, search, categories, product lookup by slug, image URL mapping. |
| `internal/service/commerce` | Cart operations with stock checks. |
| `internal/service/checkout` | Pricing (`CalculateSummary`), stock holds, draft saving, idempotent order placement. |
| `internal/service/payment` | Supported payment methods, labels, payment record construction, idempotency key generation. Stateless. |
| `internal/service/account` | A customer's orders. |
| `internal/service/admin` | Metrics, reports, order overrides, team deactivation, audit log reads. |
| `internal/service/partner` | Store settings, product creation, fulfillment, per-partner scoping. |
| `internal/db` | `Store`, connection pool, `withTx` retry helper, migrations, seed data, typed errors. `dbtest/` provisions isolated schemas for tests. |
| `internal/money` | `Cents` (int64) with parsing, formatting, and basis-point multiplication. |
| `internal/slug` | `Make` and `Valid` for URL slugs, matching the database CHECK constraints. |
| `internal/lifecycle` | `Run` (listen, serve, drain, shut down), `Readiness` (drain-aware ping), `Maintain` (background jobs). |
| `internal/bootstrap` | `Migrate`, `Seed`, `Prepare(seed bool)` over a small store interface. |
| `internal/config` | Reads environment variables into `Config` and validates them. |
| `internal/notify` | SMTP sender with STARTTLS for password-reset links. |
| `internal/ui` | Embedded `templates/` and `static/`, template parsing, `asset` cache-busting, product image fallback. |

### Service and store boundaries

Each service package that touches the database declares a private `store`
interface in its own `store.go` listing only the `*db.Store` methods it uses,
with a compile-time check such as `var _ store = (*db.Store)(nil)`. Unit tests
substitute fakes; integration tests use a real store from `dbtest`.

Services return their own error values (`partnersvc.ErrForbiddenOrderUpdate`,
`checkoutsvc.ErrCartEmpty`, ...). Handlers map those to status codes and
user-facing messages. Store errors are translated in the service, not in the
handler, except for a few `db.Err*` sentinels the API checks directly.

## Host routing

`buildServer` in `cmd/sokomoko/server.go` creates four muxes and one router:

1. The host is canonicalised (lower-cased, port stripped). Hosts not in
   `ALLOWED_HOSTS` get `400 Invalid host`.
2. Paths under `/api/v1` go to the API mux, whatever the host.
3. Hosts starting with `admin.` go to the admin mux.
4. Hosts starting with `partner.` go to the partner mux.
5. Everything else goes to the storefront mux.

Production hostnames must therefore begin with `admin.` and `partner.`. When
`ALLOWED_HOSTS` is empty (development only), the defaults are `localhost`,
`127.0.0.1`, `admin.localhost`, and `partner.localhost`.

## Request pipeline

Every request passes through this chain, outermost first
(`buildMiddlewares` in `server.go`):

| Order | Middleware | Effect |
| --- | --- | --- |
| 1 | `RequestID` | Accepts a short alphanumeric `X-Request-Id` or generates one; echoes it in the response. |
| 2 | `RequestLogger` | One structured `slog` line per request: method, path, host, status, duration, request ID. No query strings or bodies. |
| 3 | `Recoverer` | Turns panics into a 500 error page that shows the request ID. |
| 4 | `SecurityHeadersWithProxy` | CSP, `X-Frame-Options: DENY`, `nosniff`, referrer policy, `Cache-Control: no-store`, HSTS when the request is HTTPS. |
| 5 | `BodyLimit(1 MiB)` | Caps request bodies for POST, PUT, PATCH, DELETE. |
| 6 | `CSRFMiddleware` | Validates the session's CSRF token on unsafe methods (see [Security](04-security.md#csrf)). |
| 7 | `RateLimitByKey` | Only when `POST_RATE_LIMIT_MAX > 0`: fixed-window POST limit per client IP. |
| 8 | Host router | Dispatches to a mux as above. |

Per-route wrappers then add authentication: `OptionalUser` (attach the user if
signed in), `AuthMiddleware` (require a session), `RequireRole` /
`RequireAnyRole`, or `APIAuthMiddleware` for JSON routes.

## Rendering

Each page template is parsed once at startup by cloning a shared base set
(`ui.ParseTemplates`). Handlers call `app.Render` or `app.RenderStatus`, which
execute into a buffer first. A template error therefore produces a clean 500
page instead of a half-written document.

## Process lifecycle

`serve`:

1. Load and validate config. Production refuses `--seed`.
2. Build the SMTP sender if SMTP is configured.
3. Parse templates.
4. Open the store (pool sizes and query timeout from config) and ping it.
5. Prepare the schema:
   - development and test: apply pending migrations, then seed if `--seed`;
   - production: `CheckSchema` only, which fails if any embedded migration is
     missing or the database has a version the binary does not know.
6. Compose services, build the router and middleware, log a startup summary.
7. `lifecycle.Run` binds the listener, then starts `Maintain` in the
   background and serves.

`Maintain` releases expired stock reservations every minute and deletes
expired sessions and password-reset tokens every five minutes.

On `SIGINT` or `SIGTERM`, or if the listener fails, `lifecycle` marks the
readiness check as draining (`/readyz` returns 503), cancels maintenance,
and calls `http.Server.Shutdown` with a 30-second deadline. If the deadline
passes, remaining connections are closed. The store is closed only after
maintenance has returned.

HTTP server timeouts: read 15 s, read header 10 s, write 15 s, idle 60 s.

## Design principles visible in the code

- **Server-rendered first.** Every feature works as plain HTML forms. JavaScript
  enhances pages but is not required.
- **The database enforces invariants.** Money columns are integer cents with
  `CHECK` constraints; order and checkout totals must equal their parts;
  inventory cannot oversell (`reserved + allocated <= on_hand`).
- **Transactions retry.** `Store.withTx` retries serialization failures and
  deadlocks up to three times, so transaction bodies must be safe to re-run.
- **Every store call is bounded.** Store methods derive a context with the
  configured query timeout (default 10 s).
- **One binary, no runtime files.** Templates, assets, and migrations are
  embedded with `go:embed`. In development the static handler and the `asset`
  helper prefer files on disk so edits show without a rebuild.
- **Fail closed in production.** Missing hosts, database URL, reset base URL,
  or SMTP settings stop the server at startup.

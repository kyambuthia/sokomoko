# Sokomoko

Lightweight e-commerce app built with Go + SQLite + server-rendered HTML.

## Stack
- Go (`net/http`, `html/template`)
- SQLite (`github.com/ncruces/go-sqlite3`)
- Embedded + on-disk static assets (`internal/ui/static`)

## Repo Layout
```text
cmd/sokomoko/        Main server entrypoint (host-based routing)
internal/app/        App wiring and template renderer
internal/routes/     Public/admin/partner route registration
internal/auth/       Login/signup/session/auth middleware
internal/db/         Store, schema bootstrap, queries, seed data
internal/ui/         Embedded templates/static files
db/schema.sql        SQL schema reference
db/t.db              Local SQLite database (created/used at runtime)
```

## Quick Start
```bash
git clone https://github.com/kyambuthia/sokomoko.git
cd sokomoko
go mod tidy
go run ./cmd/sokomoko migrate
go run ./cmd/sokomoko seed
go run ./cmd/sokomoko serve
```

Server default: `http://localhost:6969`

For a one-step local boot with seed data:
```bash
go run ./cmd/sokomoko serve --seed
```

## Configuration
- `PORT` (default: `6969`)
- `DB_PATH` (default: `./db/t.db`)
- `ALLOWED_HOSTS` (comma-separated trusted hosts; default: `localhost,127.0.0.1,admin.localhost,partner.localhost`)
- `ENV` (`production` enables stricter cookie behavior in auth flows)
- `SESSION_COOKIE_DOMAIN` (optional; set to a shared domain such as `.example.com` to reuse login sessions across subdomains)
- `POST_RATE_LIMIT_MAX` (optional; max POST requests allowed per IP in each rate-limit window; default: `0` disabled)
- `POST_RATE_LIMIT_WINDOW_SECONDS` (optional; window size for POST rate limit; default: `60`)
- `PASSWORD_RESET_BASE_URL` (optional; absolute base URL for reset links in email, e.g. `https://shop.example.com`)
- `SMTP_HOST` / `SMTP_PORT` (optional; SMTP server host/port for password reset delivery, default port `587`)
- `SMTP_USERNAME` / `SMTP_PASSWORD` (optional; SMTP auth credentials)
- `SMTP_FROM` (optional; sender email for reset delivery, required when SMTP is enabled)

Example:
```bash
PORT=8080 \
DB_PATH=./db/t.db \
ALLOWED_HOSTS=shop.example.com,admin.example.com,partner.example.com \
SESSION_COOKIE_DOMAIN=.example.com \
POST_RATE_LIMIT_MAX=120 \
POST_RATE_LIMIT_WINDOW_SECONDS=60 \
PASSWORD_RESET_BASE_URL=https://shop.example.com \
SMTP_HOST=smtp.example.com \
SMTP_PORT=587 \
SMTP_USERNAME=mailer \
SMTP_PASSWORD=change-me \
SMTP_FROM=no-reply@example.com \
ENV=production \
go run ./cmd/sokomoko serve
```

## Domain Architecture
The server routes by host:
- `localhost:6969`: customer storefront and customer account auth
- `admin.localhost:6969`: platform/site management (admin + staff)
- `partner.localhost:6969`: store setup and partner operations

Add host entries locally:
```text
127.0.0.1 localhost
127.0.0.1 admin.localhost
127.0.0.1 partner.localhost
```

## Auth and Setup Flows
Customer (`localhost`):
- Signup: `/signup`
- Login: `/login`
- Password reset request: `/password-reset/request`
- Password reset confirm: `/password-reset/confirm`
- Account: `/account`

Admin/Staff (`admin.localhost`):
- First-run bootstrap: `/setup` (creates root admin, optional staff)
- Login: `/login`
- Staff creation (admin-only): `/staff/signup`
- Password reset request: `/password-reset/request`
- Password reset confirm: `/password-reset/confirm`
- Management dashboard/routes: `/`, `/products`, `/orders`, `/reports`, `/deliveries`

Partner (`partner.localhost`):
- Login: `/login` (admin/staff)
- Store setup: `/setup`
- Partner dashboard: `/dashboard`
- Password reset request: `/password-reset/request`
- Password reset confirm: `/password-reset/confirm`

## Runtime Notes
- `serve` applies schema, then starts the server.
- `seed` applies schema and runs bootstrap seed workflows.
- `migrate` applies schema changes without starting the server.
- First-run admin setup is done on `admin.localhost/setup` if no admin exists.
- Staff role is supported alongside admin and user.
- Password reset uses one-time, expiring reset tokens.
- Password reset can send email links via SMTP when configured (`SMTP_HOST` + `SMTP_FROM`).
- Store setup is persisted in `store_settings`.
- Static file serving checks disk first (`internal/ui/static`), then embedded assets.
- Static asset URLs are cache-busted using dynamic version query strings.
- Built-in hardening includes: security headers, panic recovery, request IDs, request logging, 1MB request body limit, same-origin checks for cookie-authenticated unsafe requests, trusted-host validation, and periodic cleanup of expired sessions/reset tokens.
- Optional IP-based POST rate limiting can be enabled via `POST_RATE_LIMIT_MAX` and `POST_RATE_LIMIT_WINDOW_SECONDS`.
- Health endpoints: `/healthz` (liveness), `/readyz` (database readiness) on public/admin/partner hosts.

## Development Commands
```bash
# Migrate schema
go run ./cmd/sokomoko migrate

# Seed bootstrap data
go run ./cmd/sokomoko seed

# Run server
go run ./cmd/sokomoko serve

# Run server with bootstrap seed
go run ./cmd/sokomoko serve --seed

# Build
go build -o bin/sokomoko ./cmd/sokomoko

# Test
go test ./...
go test -v ./...
go test -cover ./...

# Format
go fmt ./...
```

## Troubleshooting
- Port in use: set another port (`PORT=8080`).
- Host mismatch: confirm `/etc/hosts` or Windows hosts file has `admin.localhost` and `partner.localhost`.
- SQLite lock issues during local dev: stop duplicate server processes.
- Module issues: `go mod tidy`.

## Docs
- `AGENTS.md`
- `GETTING_STARTED.md`
- `docs/design/`
- `docs/reports/`

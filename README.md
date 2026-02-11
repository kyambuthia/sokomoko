# Sokomoko

Lightweight e-commerce app built with Go + SQLite + server-rendered HTML.

## Stack
- Go (`net/http`, `html/template`)
- SQLite (`github.com/ncruces/go-sqlite3`)
- Embedded + on-disk static assets (`internal/ui/static`)

## Repo Layout
```text
cmd/sokomoko/        Main server entrypoint
internal/app/        App wiring and template renderer
internal/routes/     Public/admin route registration
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
go run ./cmd/sokomoko
```

Server default: `http://localhost:6969`

## Configuration
- `PORT` (default: `6969`)
- `DB_PATH` (default: `./db/t.db`)

Example:
```bash
PORT=8080 DB_PATH=./db/t.db go run ./cmd/sokomoko
```

## Runtime Notes
- DB schema is applied automatically on startup.
- Admin bootstrap user is seeded only when `ADMIN_PASSWORD` is set and no admin exists.
- Optional bootstrap variables:
  - `ADMIN_USERNAME` (default: `admin`)
  - `ADMIN_EMAIL` (default: `admin@sokomoko.com`)
- Static file serving checks disk first (`internal/ui/static`), then embedded assets.

## Route Map
Public host (`localhost:6969`):
- `GET /`
- `GET|POST /search`
- `GET|POST /login`
- `GET|POST /signup`
- `POST /logout`
- `GET /account`
- `GET /static/*`

Admin host (`admin.localhost:6969`):
- `GET|POST /login`
- `GET /` (admin dashboard, auth required)
- `GET /products` (auth required)
- `GET /orders` (auth required)
- `GET /reports` (auth required)
- `GET /deliveries` (auth required)
- `GET /static/*`

## Optional: Local Admin Subdomain
Add to hosts file if `admin.localhost` does not resolve:
```text
127.0.0.1 localhost
127.0.0.1 admin.localhost
```

## Development Commands
```bash
# Run
go run ./cmd/sokomoko

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
- SQLite lock issues during local dev: stop duplicate server processes.
- Module issues: `go mod tidy`.

## Docs
- `AGENTS.md`
- `GETTING_STARTED.md`
- `docs/design/`
- `docs/reports/`

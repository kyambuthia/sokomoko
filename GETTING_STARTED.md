# Getting Started with Sokomoko

Contributor quick start for the current Go + SQLite + server-rendered app.

## 1. Boot the app locally

Add the local hostnames used by the app:

```text
127.0.0.1 localhost
127.0.0.1 admin.localhost
127.0.0.1 partner.localhost
```

Start the app with bootstrap data:

```bash
cd /home/mbuthi/Projects/sokomoko
go run ./cmd/sokomoko serve --seed
```

Open:

- `http://localhost:6969` for the storefront
- `http://admin.localhost:6969` for admin and staff
- `http://partner.localhost:6969` for partner setup and fulfillment

If you want schema only:

```bash
go run ./cmd/sokomoko migrate
```

If you want schema plus bootstrap data without starting the server:

```bash
go run ./cmd/sokomoko seed
```

## 2. Know the shape of the app

The app is routed by host, not by one shared URL space:

- `localhost` serves the customer storefront, cart, checkout, and account pages.
- `admin.localhost` serves admin bootstrap, admin login, dashboards, reporting, and team tools.
- `partner.localhost` serves store setup, catalog management, and fulfillment pages.

The normal request flow is:

1. `cmd/sokomoko` loads config, opens the DB, applies schema, composes the app, and starts the HTTP server.
2. `internal/app` wires the feature services, templates, static files, and middleware.
3. `internal/routes` registers host-specific handlers.
4. `internal/service/*` holds feature rules and view-facing domain logic.
5. `internal/db` owns SQL, transactions, and persistence concerns.

More detail lives in [docs/ARCHITECTURE_OVERVIEW.md](/home/mbuthi/Projects/sokomoko/docs/ARCHITECTURE_OVERVIEW.md).

## 3. Know where to make changes

- Add or change routes in `internal/routes`.
- Add business rules in `internal/service`.
- Add or change SQL in `internal/db` and keep `db/schema.sql` aligned.
- Change shared wiring and middleware in `internal/app` and `cmd/sokomoko`.
- Change pages and assets in `internal/ui/templates` and `internal/ui/static`.

If a change crosses layers, start at the service boundary and keep the route and DB changes narrow.

## 4. Run the tests that matter

Run the full suite:

```bash
go test ./...
```

Common targeted runs:

```bash
go test ./internal/auth
go test ./internal/db
go test ./internal/service/commerce
go test ./internal/service/partner
go test ./cmd/sokomoko
```

## 5. Current product and engineering priorities

The codebase is in decent shape structurally, but the next work should focus on production hardening rather than more surface-area expansion.

Top priorities:

1. Replace the current same-origin-only CSRF check with real CSRF protection for login, signup, password reset, and all unsafe form posts.
2. Add auth throttling and lockout policy for login and password reset endpoints.
3. Make admin bootstrap transactional so partial staff provisioning cannot strand credentials.
4. Add startup validation and fail-fast production checks for SMTP, cookie, and HTTPS-related configuration.
5. Replace placeholder checkout payments with a real payment flow and idempotent order submission.

The maintained backlog is in [docs/TODO.md](/home/mbuthi/Projects/sokomoko/docs/TODO.md) and the deployment view is in [docs/roadmap/DEPLOYMENT_READINESS_ROADMAP.md](/home/mbuthi/Projects/sokomoko/docs/roadmap/DEPLOYMENT_READINESS_ROADMAP.md).

## 6. Read these next

- [README.md](/home/mbuthi/Projects/sokomoko/README.md)
- [docs/ARCHITECTURE_OVERVIEW.md](/home/mbuthi/Projects/sokomoko/docs/ARCHITECTURE_OVERVIEW.md)
- [docs/authentication.md](/home/mbuthi/Projects/sokomoko/docs/authentication.md)
- [docs/TODO.md](/home/mbuthi/Projects/sokomoko/docs/TODO.md)

# Getting started

This is the shortest path from a checkout to a working local instance.

## 1. Prepare hostnames

The server selects a mux from the request host. Add these entries to the local
hosts file:

    127.0.0.1 localhost
    127.0.0.1 admin.localhost
    127.0.0.1 partner.localhost

The trusted-host default is defined in
[cmd/sokomoko/serve.go](../cmd/sokomoko/serve.go) and can be replaced with
ALLOWED_HOSTS.

## 2. Start the application

For an empty application database:

    go run ./cmd/sokomoko migrate
    go run ./cmd/sokomoko serve

For a disposable local database with sample users, categories, products, and
images:

    go run ./cmd/sokomoko serve --seed

The seed workflow reads ADMIN_PASSWORD before creating the admin seed user.
Partner seed users currently use credentials defined in
[internal/db/seed.go](../internal/db/seed.go).

## 3. Open the three surfaces

- http://localhost:6969 — storefront and customer account
- http://admin.localhost:6969 — admin setup, staff, products, orders, reports
- http://partner.localhost:6969 — store setup, catalog, and fulfillment

Health checks are available at /healthz and /readyz on every host surface.

## 4. First admin setup

When no admin user exists, open http://admin.localhost:6969/setup.
Loopback requests are accepted automatically. Non-loopback requests require
ADMIN_SETUP_TOKEN, supplied as either the X-Admin-Setup-Token header or the
setup_token form field.

The setup flow creates the root admin and can provision recommended staff
accounts. It displays generated temporary staff credentials once.

## 5. Find the right source file

| Change | Start here |
| --- | --- |
| CLI command or startup | cmd/sokomoko |
| Host routing or middleware | cmd/sokomoko/server.go, internal/app |
| URL registration | internal/routes/register.go |
| Request/page handler | internal/routes |
| Business rule | internal/service |
| SQL, transactions, or models | internal/db |
| Auth/session behavior | internal/auth |
| HTML templates | internal/ui/templates |
| CSS, JavaScript, images | internal/ui/static |

## 6. Verify changes

Use the narrowest relevant package test first, then run:

    go test ./...
    go vet ./internal/... ./cmd/...

The test suite uses SQLite files in several packages. If a test process is
interrupted, check for leftover *.db-journal files before interpreting the
working tree.

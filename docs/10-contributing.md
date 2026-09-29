# Contributor guide

This repository is a layered Go application. Keep changes close to the layer
that owns the behavior.

## Source order

1. cmd/sokomoko — commands and process lifecycle
2. internal/app — composition and middleware
3. internal/routes — HTTP methods, auth wrappers, page assembly
4. internal/service — business rules
5. internal/db — SQL, models, and transactions
6. internal/ui — templates and static assets

Do not document behavior from an unused template, stale SQL snapshot, or
historical report. Verify the live route registration and template parser
first.

## Local checks

    gofmt -w ./cmd ./internal
    go test ./...
    go vet ./internal/... ./cmd/...

Prefer focused tests while developing:

    go test ./internal/auth
    go test ./internal/db
    go test ./internal/routes
    go test ./internal/service/checkout
    go test ./cmd/sokomoko

## Implementation rules

- Check HTTP methods explicitly in handlers.
- Put validation and state transitions in services, not templates.
- Use parameterized SQL and transactions for multi-step writes.
- Keep money rounding and checkout totals in checkout code.
- Add CSRF fields to every authenticated unsafe form.
- Use Auth.CSRFToken for JavaScript requests carrying the session cookie.
- Update the canonical embedded schema at internal/db/schema.sql when the
  database model changes.
- Add tests where the rule lives.
- Reuse existing template base files and CSS tokens before adding new patterns.

## Runtime notes

- The application uses host-based routing.
- The default database is ./db/t.db.
- Schema application is idempotent, but incremental migrations are not yet
  implemented.
- serve --seed is intended for disposable development data.
- Do not commit local SQLite databases, journal files, generated assets, or
  credentials.

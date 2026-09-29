# Architecture

Sokomoko is a single Go binary with one SQLite store and server-rendered HTML.
The runtime is composed in
[internal/app/compose.go](../internal/app/compose.go).

## Runtime sequence

runServe in [cmd/sokomoko/serve.go](../cmd/sokomoko/serve.go):

1. Configures the optional password-reset email sender.
2. Parses the templates.
3. Opens SQLite.
4. Applies the embedded schema and optionally runs seed workflows.
5. Composes the application services.
6. Registers public, admin, and partner muxes.
7. Wraps the host router in shared middleware.
8. Starts the HTTP server and background cleanup loop.

migrate applies the schema and exits. seed applies the schema, runs all seed
workflows, and exits.

## Request path

    HTTP request
      → trusted-host check
      → recovery, request ID, security headers, body limit
      → CSRF token validation for cookie-authenticated unsafe methods
      → optional in-memory IP limiter
      → request logging
      → public/admin/partner mux
      → route handler
      → feature service
      → SQLite query or transaction
      → HTML template or redirect

Host routing is implemented in
[cmd/sokomoko/server.go](../cmd/sokomoko/server.go). Hosts beginning with
admin. use the admin mux; hosts beginning with partner. use the partner mux;
all other allowed hosts use the public mux.

## Package responsibilities

| Package | Responsibility |
| --- | --- |
| cmd/sokomoko | CLI commands, startup, host routing, shutdown |
| internal/app | dependency container, composition, middleware, shared errors |
| internal/auth | users, passwords, sessions, CSRF, reset flows, role guards |
| internal/routes | route registration, method checks, page handlers |
| internal/service/catalog | product and category reads |
| internal/service/commerce | cart reads and mutations |
| internal/service/checkout | checkout snapshots, inventory reservation, totals, placement |
| internal/service/payment | supported methods and payment record construction |
| internal/service/account | customer account and order history |
| internal/service/admin | metrics, products, orders, reports, teams, audit |
| internal/service/partner | store settings, catalog operations, fulfillment |
| internal/db | embedded schema, queries, transactions, models, seed data |
| internal/ui | embedded templates, static files, asset URLs |
| internal/notify | SMTP password-reset delivery |

## Data and transaction boundaries

SQLite is opened with foreign keys enabled, a five-second busy timeout, and a
maximum pool size of ten connections. Checkout inventory reservation uses a
pinned connection and BEGIN IMMEDIATE. Final order placement creates the order,
order lines, payment, payment attempt, inventory allocation, checkout
completion, idempotency record, and cart clear inside one transaction.

The checkout flow is:

1. Read the current cart.
2. Reserve stock for fifteen minutes.
3. Persist checkout lines and a totals snapshot.
4. Accept an address, payment method, and idempotency key.
5. Reuse an existing placement for a repeated key.
6. Place the order transactionally.

Current totals are defined in
[internal/service/checkout/checkout.go](../internal/service/checkout/checkout.go):
$6.50 shipping below $80, free shipping at or above $80, and estimated tax
of 8%.

## Schema authority

The runtime schema is the embedded
[internal/db/schema.sql](../internal/db/schema.sql), loaded by
[internal/db/store.go](../internal/db/store.go). The separate
[db/schema.sql](../db/schema.sql) file is an older, incomplete snapshot and
is not read by the application. Do not treat the two files as interchangeable.

Schema version 2 is recorded in schema_version, but incremental migration steps
are not implemented. Existing databases receive a compatibility migration for
the sessions.csrf_token column.

## Templates and assets

Templates are parsed explicitly in
[internal/ui/templates.go](../internal/ui/templates.go). Shared base
templates are combined with page templates at startup. Static requests try
internal/ui/static on disk first and then fall back to the embedded asset
filesystem. Asset URLs receive a modification-time or content-hash query
parameter.

## Design constraints

- Keep transport logic in internal/routes.
- Keep business rules in internal/service.
- Keep SQL and transaction details in internal/db.
- Add template data fields at the route/view boundary.
- Preserve host routing unless changing the public surface deliberately.

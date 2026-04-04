# Architecture Overview

Last updated: April 4, 2026

## What Sokomoko is

Sokomoko is a host-routed, server-rendered e-commerce app built with Go, `net/http`, HTML templates, embedded static assets, and SQLite.

It is one binary, one database, and three host surfaces:

- `localhost`: customer storefront, cart, checkout, and account flows
- `admin.localhost`: admin bootstrap, login, reporting, team management, and order operations
- `partner.localhost`: partner setup, product management, and fulfillment flows

## Runtime composition

Startup lives in [`cmd/sokomoko`](/home/mbuthi/Projects/sokomoko/cmd/sokomoko).

`runServe` does four things:

1. Load templates and static assets.
2. Open the SQLite store.
3. Apply schema and optional bootstrap seed through `bootstrap.New(store).Prepare(...)`.
4. Compose the app and register the three host-specific muxes.

The composition root is [`internal/app/compose.go`](/home/mbuthi/Projects/sokomoko/internal/app/compose.go). It wires these services into one `app.App`:

- catalog
- account
- commerce
- admin
- partner
- auth

## Request flow

The normal request path is:

1. `cmd/sokomoko/server.go` picks the host mux.
2. `internal/app` middleware applies request ID, security headers, body limits, CSRF same-origin checks, optional POST rate limiting, and request logging.
3. `internal/routes` handles transport details and page assembly.
4. `internal/service/*` enforces feature rules and translates store data into route-facing models.
5. `internal/db/*` performs SQL reads, writes, and transactions.
6. `internal/ui` renders HTML templates and serves static assets.

Keep transport concerns in routes, domain rules in services, and SQL-only concerns in the DB layer.

## Package responsibilities

### `cmd/sokomoko`

- CLI entrypoints and command dispatch
- startup logging
- server construction
- host-based routing and middleware assembly

### `internal/app`

- application dependency container
- middleware
- shared error page rendering
- composition tests

### `internal/auth`

- customer login/signup
- admin bootstrap and admin or staff login
- password reset request and confirm flows
- session cookie issuance and auth middleware

### `internal/routes`

- public, admin, and partner route registration
- host-specific page handlers
- response shaping for templates

### `internal/service`

- `account`: customer order history
- `admin`: metrics, team management, audit logs, order state updates
- `catalog`: storefront product browsing and search
- `commerce`: cart and checkout rules
- `payment`: supported payment methods, idempotency keys, and payment-record construction
- `partner`: store setup, partner catalog actions, fulfillment updates

### `internal/db`

- schema application
- SQL models and query helpers
- cart and order transaction logic
- user, session, audit, and settings persistence

### `internal/ui`

- template parsing
- embedded assets
- CSS, JS, icons, and images

## Data model summary

Core tables:

- `users`
- `sessions`
- `password_reset_tokens`
- `categories`
- `products`
- `product_images`
- `carts`
- `cart_items`
- `orders`
- `payments`
- `payment_attempts`
- `idempotency_keys`
- `order_items`
- `store_settings`
- `audit_logs`

Schema changes must update both:

- [`db/schema.sql`](/home/mbuthi/Projects/sokomoko/db/schema.sql)
- [`internal/db/schema.sql`](/home/mbuthi/Projects/sokomoko/internal/db/schema.sql)

## Current strengths

- Clear separation between route, service, and DB layers
- Good test coverage around auth, bootstrap, route registration, and service logic
- Explicit startup commands and centralized app composition
- Basic operational hardening already present: request IDs, security headers, body limits, panic recovery, trusted-host routing, and expired session or token cleanup

## Current gaps that should drive work

These are the next areas worth investing in:

1. Auth hardening: real CSRF tokens, login throttling, lockouts, verified email, optional MFA.
2. Bootstrap safety: admin setup should be transactional so partial staff provisioning cannot leave hidden state.
3. Production startup validation: fail fast on invalid SMTP or unsafe production config instead of degrading silently.
4. Checkout correctness: real payments, payment state tracking, and idempotent checkout submission.
5. Operations: structured logs, metrics, backup or restore docs, and deployment checks.

For the larger commerce-domain evolution plan, see [docs/roadmap/GO_COMMERCE_EVOLUTION_PLAN.md](/home/mbuthi/Projects/sokomoko/docs/roadmap/GO_COMMERCE_EVOLUTION_PLAN.md).

## Working rules for contributors

- Prefer changing one layer at a time and keep interfaces explicit.
- Add tests in the layer where the rule actually lives.
- Do not change schema without updating both schema files and relevant DB tests.
- Reuse existing templates and CSS primitives before introducing new view patterns.
- Preserve the host-based architecture unless there is a deliberate routing redesign.

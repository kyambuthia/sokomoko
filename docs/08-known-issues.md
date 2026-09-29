# Known issues and source-backed backlog

This list describes current behavior found in the executable source. It is
ordered by impact, not by historical implementation phase.

## 1. Fix incomplete CSRF integration

The live middleware requires a session CSRF token for every unsafe request with
a session cookie. Several live forms do not render one:

- storefront add-to-cart forms in index, product, and search pages
- logout in account.html
- admin order updates and team deactivation
- partner store setup
- partner fulfillment updates

The quick-search JavaScript POST also does not send X-CSRF-Token. The
middleware is correct; the templates and client request need to use the token
helpers in internal/auth/csrf.go.

## 2. Make the request rate limiter honor its method argument

RateLimitByIP accepts a list of methods, and startup passes POST, but the
implementation currently limits every request. This can throttle GET, static,
and health traffic when POST_RATE_LIMIT_MAX is enabled.

Source: [internal/app/rate_limit.go](../internal/app/rate_limit.go).

## 3. Resolve partner identity and scoping

The schema and seed workflow create role partner users, but the admin login
handler accepts only admin and staff. The partner host therefore serves
admin/staff users. Partner catalog and order queries are also global rather
than scoped to the authenticated partner.

Choose one model, then align roles, login, route guards, product ownership,
orders, and seed data.

## 4. Make admin bootstrap transactional

Admin setup creates the root admin and staff users through sequential writes.
A partial failure can leave an admin present while staff provisioning is
incomplete, which prevents the setup page from being used again.

Source: AdminSetup in [internal/auth/auth.go](../internal/auth/auth.go).

## 5. Replace placeholder payment behavior

The checkout flow already has idempotency keys and transactional order
placement. Card and mobile-money methods are still placeholders that create
captured records without a provider. Cash on delivery remains pending.

Implement provider state transitions, callbacks, retries, reconciliation, and
refund behavior before production payment use.

## 6. Remove or reconcile stale source artifacts

- db/schema.sql is an incomplete duplicate of internal/db/schema.sql.
- Several page templates are not parsed by internal/ui/templates.go.
- Generated SQLite journal files should not be committed.

Either remove these artifacts or clearly mark them as intentionally retained.

## 7. Improve production operations

Remaining operational gaps include startup validation, database backup and
restore procedures, structured logs, metrics, dependency auditing, and a
production smoke test.

## Verified foundations

The source already contains:

- hashed session IDs and password-reset tokens
- per-session CSRF tokens
- role middleware
- host allowlisting
- panic recovery and security headers
- request IDs and body limits
- inventory reservations and transactional order placement
- checkout idempotency records
- periodic cleanup for expired sessions and reset tokens

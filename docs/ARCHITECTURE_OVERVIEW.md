# Architecture Overview

Last updated: October 5, 2026

## What Sokomoko is

Sokomoko is a host-routed, server-rendered marketplace built with Go,
`net/http`, `html/template`, embedded static assets, and PostgreSQL.

It is one binary, one database, and three host surfaces:

- `localhost`: customer storefront, cart, checkout, and account
- `admin.localhost`: first-run admin setup, reporting, team management, order operations, audit log
- `partner.localhost`: store setup, catalog entry, and fulfillment

## Runtime composition

Startup lives in `cmd/sokomoko`. `serve`:

1. Loads and validates configuration (production refuses unsafe or missing settings).
2. Configures `slog` (JSON in production); the standard `log` package routes through it.
3. Parses templates and opens a pooled PostgreSQL connection.
4. Applies pending migrations, and seeds demo data with `--seed`.
5. Composes the app (`internal/app/compose.go`) and registers the three host muxes.
6. Starts background jobs: reservation expiry (every minute) and session/reset-token pruning (every five minutes).
7. Serves until SIGINT/SIGTERM, then shuts down gracefully.

## Request flow

1. `cmd/sokomoko/server.go` validates the `Host` header and picks the host mux.
2. Middleware in order: panic recovery, request ID (inbound IDs validated), security headers (CSP, HSTS over TLS), 1 MB body limit, CSRF token check, optional POST rate limit (proxy-aware client IP), structured access log.
3. `internal/routes` handles transport: parsing, flash messages, status codes, JSON vs HTML responses.
4. `internal/service/*` enforces feature rules and maps store data into view models.
5. `internal/db` runs SQL inside bounded-timeout contexts and transactions.
6. `internal/ui` renders templates; `App.Render` buffers output so a template error yields a clean 500 page.

Keep transport concerns in routes, domain rules in services, and SQL in the DB layer.

## Frontend model

Pages are fully server-rendered and work without JavaScript. Custom elements in
`internal/ui/static/scripts/components.js` enhance server markup in place:

| Element | Enhances |
| --- | --- |
| `ui-cart-count` | Header cart badge from `GET /api/cart`, updated live on cart events |
| `ui-add-to-cart` | `/cart/add` forms: add without a page reload, toast feedback |
| `ui-cart-line` | Cart quantity inputs: debounced updates of line totals and subtotal |
| `ui-confirm` | Destructive forms: confirmation prompt |
| `ui-countdown` | Checkout stock-hold countdown |
| `ui-relative-time` | Timestamps rendered as "5 minutes ago" |
| `ui-order-status` | Order progress tracker polling `GET /api/orders/status` |
| `ui-table-filter` | Client-side filtering of admin and partner tables |
| `ui-copy` | Copy-to-clipboard for one-time credentials |
| `ui-char-counter` | Remaining characters for length-limited fields |
| `ui-category-nav` | Department menu from `GET /api/categories` |

`main.js` holds the earlier presentational components (alerts, badges, price,
quantity stepper, header search, toasts, dialogs). Cart endpoints return JSON when
the request sends `Accept: application/json`, and redirect with a flash message
otherwise. JavaScript sends the CSRF token from the enhanced form in the
`X-CSRF-Token` header.

## Data model

All tables are defined in `internal/db/migrations`. Conventions: `BIGINT`
identity keys, `TIMESTAMPTZ` timestamps, money in `*_cents` `BIGINT` columns
(`internal/money.Cents` in Go), and `updated_at` maintained by triggers.

- Identity: `users` (case-insensitive unique username and email, soft delete), `sessions` (SHA-256 of the cookie token plus a CSRF token), `password_reset_tokens` (hashed, single use).
- Marketplace: `store_settings` (singleton), `partners` (vendors).
- Catalog: `categories`, `products` (full-text `search_vector` with a GIN index, partial unique slug), `product_images`.
- Inventory: `warehouses`, `inventory_stocks` (single source of truth; `CHECK reserved + allocated <= on_hand`), `stock_reservations` (checkout holds with expiry), `stock_movements` (append-only ledger).
- Shopping: `carts`, `cart_items`, `checkouts` and `checkout_lines` (priced snapshot of the cart).
- Orders: `orders` (status, partner fulfillment status, delivery status, `inventory_state`, totals that must add up), `order_items` (price and partner snapshot, generated line totals).
- Payments: `payments`, `payment_attempts`, `idempotency_keys`.
- Audit: `audit_logs`.

### Stock lifecycle

```text
available = on_hand - reserved - allocated

checkout prepared   reserved  += qty                (stock_reservations active)
hold expires        reserved  -= qty                (background job)
cart changes        reserved  -= qty, checkout cancelled
order placed        reserved  -= qty, allocated += qty   (inventory_state = allocated)
order dispatched    allocated -= qty, on_hand -= qty     (inventory_state = shipped)
order cancelled     allocated -= qty                     (inventory_state = released)
```

Each transition writes a `stock_movements` row. Inventory rows are locked
`FOR UPDATE` in product-id order to avoid deadlocks, and transactions retry on
serialization failures and deadlocks.

### Checkout and idempotency

The checkout token doubles as the idempotency key. Submitting the same token
again returns the original order. Changing the cart cancels the open checkout
and releases its holds, so an order is never placed for a stale snapshot.

## Testing

- Unit tests sit next to the code. DB-backed tests use `internal/db/dbtest`, which creates a fresh PostgreSQL schema per package from `SOKOMOKO_TEST_DATABASE_URL`.
- `cmd/sokomoko` integration tests drive the full HTTP stack, including CSRF, host routing, and role checks.
- `internal/db/orders_test.go` includes a concurrent oversell test; run it under `-race`.

## Working rules

- Change one layer at a time and keep interfaces explicit (each service declares the `store` interface it needs).
- Add tests in the layer where the rule lives.
- Schema changes go in a new numbered migration. Never edit an applied one.
- Every state-changing form needs a `csrf_token` field. Page data structs carry `CSRFToken`.
- New dynamic behavior should enhance server-rendered markup, not replace it.
- Preserve the host-based architecture unless there is a deliberate routing redesign.

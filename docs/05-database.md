# Database and commerce state

PostgreSQL is accessed through `database/sql` and pgx v5. Versioned migrations in
`internal/db/migrations` are embedded in the binary and applied transactionally
under an advisory lock. Production `serve` verifies migration history; the
separate `migrate` command applies changes. Never modify applied migrations.

Pool and query limits are configured with `DB_MAX_OPEN_CONNS`,
`DB_MAX_IDLE_CONNS`, `DB_CONN_MAX_LIFETIME_SECONDS`, and `DB_QUERY_TIMEOUT_SECONDS`.
Money is stored as integer cents and computed using `internal/money.Cents`.
The versioned API converts amounts to numeric major units at its boundary.

## Tables by concern

### Identity and configuration

- users
- sessions
- password_reset_tokens
- store_settings
- schema_migrations
- partners
- partner_users (explicit login-account membership)

User roles allowed by the live schema are user, staff, admin, and partner.
Store settings are a single row identified by id 1.

### Catalog

- categories
- products
- product_images

Products have a unique slug, price_cents, derived available-stock projection, optional
category, and optional partner_id. Soft-delete timestamps exist on users,
categories, and products.

### Inventory

- warehouses
- inventory_stocks
- stock_reservations
- stock_movements

The default warehouse is created with id 1. Inventory tracks on-hand,
reserved, and allocated quantities. Reservations have active, released,
expired, and converted states. Movements record initial, adjustment,
reservation, release, allocation, and return changes.

Available stock is computed from inventory on hand minus reserved and allocated
quantities. Inventory rows are locked in product-ID order for mutations.

### Commerce

- carts
- cart_items
- checkouts
- checkout_lines
- orders
- order_items
- payments
- payment_attempts
- idempotency_keys

Cart ownership is one cart per user. Checkout lines snapshot product name,
quantity, unit price, line total, and reservation key. Orders snapshot product
name and unit price in order_items.

Order status fields are:

- status: pending, processing, shipped, delivered, cancelled
- partner_status: new, accepted, packing, dispatched, completed, cancelled
- delivery_status: queued, processing, shipped, delivered

Payment status fields are pending, captured, failed, and refunded. Payment
attempts are pending, completed, or failed.

## Checkout transaction

The checkout service uses a fifteen-minute reservation hold:

1. Reserve the current cart in a PostgreSQL transaction with inventory row locks.
2. Expire stale reservations and release the user's previous active hold.
3. Persist a checkout snapshot with subtotal, shipping, tax, and total.
4. On submit, validate address and supported payment method.
5. Look up the user's idempotency key.
6. Place the order and convert the reservation to allocation in one transaction.
7. Create the payment and payment attempt, complete the checkout, clear the
   cart, and finish the idempotency record.

Totals are currently:

- flat shipping: $6.50
- free shipping threshold: $80.00
- estimated tax: 8%
- currency: USD

## Seed workflows

The bootstrap service runs:

1. schema application
2. admin seed
3. partner seed
4. initial catalog seed

Admin seed requires ADMIN_PASSWORD and optionally reads ADMIN_USERNAME and
ADMIN_EMAIL. Demo partner records and catalog data are development-only; partner login
accounts require explicit membership in `partner_users`. Production rejects seeding.

## Database commands

    go run ./cmd/sokomoko migrate
    go run ./cmd/sokomoko seed

Tests create isolated PostgreSQL schemas using `SOKOMOKO_TEST_DATABASE_URL`.
Set `SOKOMOKO_REQUIRE_DB_TESTS=1` to fail instead of skipping when it is absent.

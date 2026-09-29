# Database and commerce state

The application uses SQLite through
[internal/db/store.go](../internal/db/store.go). The live schema is embedded
from [internal/db/schema.sql](../internal/db/schema.sql).

## Connection behavior

Opening the store:

- enables foreign keys
- sets a five-second busy timeout
- limits the pool to ten open and ten idle connections
- keeps connections alive for the process lifetime

Applying the schema is idempotent. It also adds sessions.csrf_token to legacy
databases that do not have that column and records schema version 2.
Incremental migration steps are not implemented.

The root db/schema.sql file is an older snapshot and is not loaded at runtime.
It does not include all current schema features, so changes should target the
embedded internal/db/schema.sql file.

## Tables by concern

### Identity and configuration

- users
- sessions
- password_reset_tokens
- store_settings
- schema_version

User roles allowed by the live schema are user, staff, admin, and partner.
Store settings are a single row identified by id 1.

### Catalog

- categories
- products
- product_images

Products have a unique slug, price, stock_quantity projection, optional
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

products.stock_quantity is synchronized from inventory operations; the
inventory tables are the operational source for reservation and allocation
behavior.

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

1. Reserve the current cart in an immediate SQLite transaction.
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
ADMIN_EMAIL. Partner seed accounts use fixed credentials in
[internal/db/seed.go](../internal/db/seed.go), which makes the seed workflow
appropriate only for disposable development data.

## Database commands

    go run ./cmd/sokomoko migrate
    go run ./cmd/sokomoko seed

Tests create temporary SQLite stores and exercise the same schema path used by
the application.

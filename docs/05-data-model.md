# Data model

PostgreSQL 17 is the only data store. All SQL lives in `internal/db`, and the
schema is defined by the embedded migrations in `internal/db/migrations/`.

## Schema conventions

From the header of `0001_initial_schema.sql`:

- Keys are `BIGINT GENERATED ALWAYS AS IDENTITY`.
- Timestamps are `TIMESTAMPTZ DEFAULT now()`. The `set_updated_at()` trigger
  maintains `updated_at`.
- Money is `BIGINT` cents in `*_cents` columns, with `CHECK (>= 0)`.
- Users, products, and categories are soft-deleted with `deleted_at`. Unique
  indexes on slugs and names apply to active rows only.
- Rows with financial or audit history are not cascade-deleted from their user
  (`orders.user_id` and `payments.user_id` are `ON DELETE RESTRICT`).

## Tables

| Area | Table | Purpose |
| --- | --- | --- |
| Identity | `users` | Accounts. `role` is `user`, `staff`, `admin`, or `partner` (`partner` added in migration 2). |
| | `sessions` | SHA-256 of the session token, user, CSRF token, expiry |
| | `password_reset_tokens` | SHA-256 of the token, expiry, `used_at` |
| | `partner_users` | Links a `partner` user to one partner (migration 2) |
| Marketplace | `store_settings` | Single row (`id = 1`): store name, slug, description, contact email |
| | `partners` | Vendors: name, slug, contact email, `active` or `suspended` |
| Catalog | `categories` | Name, slug, optional parent |
| | `products` | Partner, category, name, slug, description, `price_cents`, currency, generated `search_vector` |
| | `product_images` | URL, alt text, position |
| Inventory | `warehouses` | One default warehouse is created by the migration |
| | `inventory_stocks` | Per product and warehouse: `on_hand`, `reserved`, `allocated` |
| | `stock_reservations` | Checkout holds with status and expiry |
| | `stock_movements` | Append-only ledger of every stock change |
| Cart | `carts`, `cart_items` | One cart per user; line quantity 1 to 999 |
| Checkout | `checkouts`, `checkout_lines` | Priced snapshot of the cart with a unique token |
| Orders | `orders`, `order_items` | Placed orders with three status fields and an inventory state |
| Payments | `payments`, `payment_attempts` | Payment record per order; one attempt per provider request, kept after retries (migrations 3 and 4) |
| | `commerce_settings` | Single row: store currency, flat shipping, free-shipping threshold, tax rate (migration 3) |
| | `payment_providers` | Enabled flag, mode, and encrypted credentials per provider (migration 3) |
| | `payment_exceptions` | Confirmed charges that need a refund: late captures and duplicate charges, with refund status (migration 6) |
| | `idempotency_keys` | Unique `(user_id, operation, key)` that makes order placement idempotent |
| Audit | `audit_logs` | Actor, action, target, details |
| Migrations | `schema_migrations` | Applied versions (created by the migration runner) |

Constraints that encode business rules:

- `inventory_stocks_not_oversold`: `reserved + allocated <= on_hand`.
- `orders_total_matches` and `checkouts_total_matches`:
  `total = subtotal + shipping + tax`.
- `order_items.line_total_cents` and `checkout_lines.line_total_cents` are
  generated columns (`unit_price * quantity`).
- `warehouses_single_default_key`: at most one default warehouse.

## Migrations

`internal/db/migrate.go`:

- Files are named `<version>_<name>.sql` and embedded with `go:embed`.
  Duplicate or non-numeric versions fail at load time.
- `Migrate` takes a PostgreSQL advisory lock, creates `schema_migrations` if
  needed, and applies each pending file in its own transaction together with
  its history row.
- `CheckSchema` (used by production `serve`) runs no DDL. It fails when any
  embedded version is missing from the database, or when the database records a
  version the binary does not have.
- `LatestSchemaVersion` reports the newest embedded version (logged at startup).

To change the schema, add a new numbered file. Never edit an applied migration.
Keep changes additive so the previous binary keeps working during a release.

## Money

`internal/money.Cents` is an `int64` count of cents.

- `Parse` reads user input such as `12`, `12.5`, `$12.50`. Negative values,
  more than two fraction digits, and non-numeric input are rejected.
- `String()` formats as `12.50`; `Float64()` is used only at the JSON edge.
- `MulBasisPoints(800)` computes 8%, rounding half away from zero (used for
  tax).

No floating-point arithmetic is used for prices or totals.

## Inventory model

Available stock for a product is `on_hand - reserved - allocated` in the
default warehouse. Every change writes a `stock_movements` row.

```text
                reserve (checkout)            place order
 available ───────────────────────▶ reserved ─────────────▶ allocated
     ▲            release / expire     │                       │
     └─────────────────────────────────┘                       │
     ▲                       cancel before shipping            │ dispatch / deliver
     └─────────────────────────────────────────────────────────┤
                                                               ▼
                                                    shipped (on_hand drops)
```

- **Reserve.** `ReserveCartForCheckout` locks each stock row, checks
  availability, raises `reserved`, and records a reservation that expires in
  15 minutes. A user's earlier holds are released first, so each user holds
  stock for at most one checkout.
- **Release.** Any cart change calls `invalidateCheckoutsTx`, which releases
  the user's holds and cancels open checkouts. `Maintain` expires lapsed holds
  every minute.
- **Allocate.** Placing an order converts each reservation (`status =
  'converted'`) and moves its quantity from `reserved` to `allocated`.
- **Ship or release.** Order status changes call
  `applyOrderInventoryTransition`. Moving to shipped lowers both `on_hand` and
  `allocated`; cancelling lowers `allocated`. Both are terminal, and the
  transition is idempotent.

Stock rows are always locked in product ID order to avoid deadlocks.

## Checkout and order placement

```text
GET /checkout          PreparedCheckout
                         ├─ resume latest open checkout, or
                         └─ Prepare: reserve stock, price cart, UpsertCheckout (token)
POST /checkout         CheckoutWithPayment(token)
                         ├─ token already completed? return that order
                         ├─ ensureOpen: re-prepare if the checkout lapsed
                         ├─ payment.BuildRecord (cash on delivery, pending)
                         └─ PlaceOrderFromCheckoutWithPayment (one transaction)
                              1. insert idempotency key (concurrent duplicates serialize here)
                              2. return the existing placement if the key is complete
                              3. lock the checkout; reject expired or non-open
                              4. createOrderTx: order, items, reservation -> allocation
                              5. insert the payment
                              6. complete the checkout and the idempotency key
                              7. clear the cart
```

The checkout token is the idempotency key. Submitting the same form twice, or
calling `place-order` twice, returns the same order. Prices come from the
checkout snapshot, not live product rows, so a price change after the checkout
was prepared does not change what the customer pays.

`SaveDraft` stores the delivery address (trimmed to 500 characters) and payment
method, so a failed submit does not lose what the customer typed.

## Order state machine

An order has three status fields and one inventory state:

| Field | Values |
| --- | --- |
| `status` | `pending`, `processing`, `shipped`, `delivered`, `cancelled` |
| `partner_status` | `new`, `accepted`, `packing`, `dispatched`, `completed`, `cancelled` |
| `delivery_status` | `queued`, `processing`, `shipped`, `delivered` |
| `inventory_state` | `allocated`, `shipped`, `released` |

**Partner updates** (`UpdateOrderFulfillment`) drive the order:

| Partner status | Allowed next | Sets `status` to |
| --- | --- | --- |
| `new` | `accepted`, `cancelled` | `pending` |
| `accepted` | `packing`, `cancelled` | `processing` |
| `packing` | `dispatched`, `cancelled` | `processing` |
| `dispatched` | `completed` | `shipped` |
| `completed` | (terminal) | `delivered` |
| `cancelled` | (terminal) | `cancelled` |

Delivery status may only move forward (`queued` -> `processing` -> `shipped`
-> `delivered`, skipping allowed). Staying in the same state is always allowed.

**Admin updates** (`UpdateOrderByAdmin`) may set any valid combination, with one
guard: an order whose stock was released (cancelled) cannot be reopened.

**Customer cancellation** (`CancelOrderByCustomer`) moves an order to
`cancelled` only while `status` is `pending`, `partner_status` is `new`, and no
payment is captured or refunded. It locks the payments before the order (the
same order `CapturePayment` uses), releases stock, and fails open payment
attempts. A charge confirmed afterwards becomes a late capture in
`payment_exceptions` ([Payments](payments.md)).

Every partner and admin update writes an `audit_logs` row.

## Search

`products.search_vector` is a stored `tsvector` built with the `simple`
configuration: name weighted A, description weighted B. It has a GIN index.
`ListProducts` matches `websearch_to_tsquery` or a case-insensitive substring
on name or description, so partial words still match. Results are ranked with
`ts_rank`. Category filters match a category slug or name.

## Product edits

`UpdateProduct` changes name, description, price, and category, and sets
*available* stock: `on_hand` becomes the new figure plus whatever is reserved
or allocated, so open checkouts and placed orders keep their stock. The change
is written to `stock_movements` as an adjustment. The slug never changes.
Checkouts snapshot prices, so a price edit does not change what a customer
already in checkout pays.

## Seed data

`internal/db/seed.go`, run by `seed` and `serve --seed`:

- `SeedAdmin`: creates an admin from `ADMIN_*` variables only when
  `ADMIN_PASSWORD` is set and no admin exists.
- `SeedPartners`: four demo partners.
- `SeedInitialCatalog`: demo categories and products with images. Existing
  products are left alone.

All three are idempotent.

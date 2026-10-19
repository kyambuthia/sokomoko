# Known limitations

Gaps visible in the code as of this writing, grouped by area. Each one names
where to look. Planned work is tracked in [Roadmap](roadmap.md).

## Commerce

- **Flat pricing.** One store currency, one flat shipping fee with an optional
  free-shipping threshold, and one estimated tax rate (`/payments` on the admin
  host). There is no tax by region, shipping zones, or discounts.
- **One warehouse.** The schema supports several, but every read and write uses
  the default warehouse.
- **Mixed-vendor orders.** A partner user can see and update only orders whose
  every line is theirs. Orders that mix partners can be fulfilled only by admin
  or staff.
- **Customers cannot cancel paid orders.** Self-service cancellation stops once
  the vendor accepts the order or a payment is captured
  (`db.CancelOrderByCustomer`). After that, admin cancels and refunds.
- **M-Pesa refunds are manual.** The operator reverses the charge in the M-Pesa
  portal and records the receipt. Only Paystack refunds are requested by the
  server ([Payments](payments.md)).

## Catalog and vendor management

- **No product deletion or image upload.** `DeleteProduct` and
  `CreateProductImage` exist in `internal/db` but are called only by seed code
  and tests. New products show a bundled fallback image.
- **A product's partner cannot be changed after creation.**

## Identity

- No email verification and no multi-factor authentication. Profile details
  (username, email) cannot be changed while signed in; `UpdateUser` has no
  callers outside tests.
- Admin accounts cannot be deactivated or reactivated from the team page.

## Operations

- **Rate limits are per instance.** POST rate limits and auth lockouts live in
  process memory, so running several replicas weakens them. Maintenance jobs
  are safe to run on every replica: each pass takes a PostgreSQL advisory lock
  and only one replica does the work.
- **Password-reset email has no outbox.** It is sent from a goroutine and
  retried twice on transient failures (5-second timeout per attempt); a
  process restart during that window loses it.
- **No metrics or tracing endpoint.** Only structured logs and the health
  endpoints.

## Performance

- The admin team page lists every account, customers included, without
  pagination. The storefront account page loads every order of the customer;
  the JSON API pages both in SQL.

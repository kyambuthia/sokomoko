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

- **No product deletion or image upload.** `db.DeleteProduct` exists but only
  tests call it; product images are written only with a new product (seed
  data). New products show a bundled fallback image.
- **A product's partner cannot be changed after creation.**

## Services and local discovery

- Service listings support discovery and seller management. They do not reserve
  appointments, manage practitioner calendars, take deposits or confirm bookings.
- Locality search matches seller-entered neighbourhood/town text. There is no
  geocoding, map display, distance search or live location tracking.
- Professional qualifications are not verified. The directory does not collect
  clinical details, and a dental category is not an endorsement.
- Service enquiries use the existing encrypted shop messaging flow. Listing
  images and appointment-specific conversations are not implemented yet.

## Identity

- No email verification and no multi-factor authentication. Profile details
  (username, email) cannot be changed while signed in; `UpdateUser` has no
  callers outside tests.

## Operations

- **Rate-limit fallbacks are per instance.** POST limits and authentication
  lockouts use PostgreSQL when it is available, but use local counters when a
  database call fails, so replicas can each allow additional attempts during an
  outage. Auth lockout state uses a logged table and follows database replication
  durability settings; POST counters are unlogged and can reset after a crash or
  failover. Maintenance
  jobs use an advisory lock, so one replica performs each pass.
- **Password-reset email has no outbox.** It is sent from a goroutine and
  retried twice on transient failures (5-second timeout per attempt); a
  process restart during that window loses it.
- **No metrics or tracing endpoint.** Only structured logs and the health
  endpoints.

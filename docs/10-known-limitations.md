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

- **Limited product media management.** The partner workspace accepts one
  optional JPEG or PNG image (up to 900 KB before normalization to 256 KB) and
  one MP4 or WebM video (up to 10 MiB), stored in PostgreSQL. New uploads
  replace the previous uploaded file of that type; leaving a file input empty
  keeps the current upload. Sellers can remove uploaded media. Images cannot
  be reordered and videos are served as uploaded, without transcoding. Products
  without an image show a bundled fallback. Deleting a product removes its
  image and video bytes; past orders retain product records.
- **A product's partner cannot be changed after creation.**

## Services and local discovery

- Appointments use a request-to-book flow. Requests are checked against the
  listing's timezone and weekly opening hours, and overlapping requested,
  confirmed or in-progress appointments are blocked for each default
  capacity-one resource. Customers can cancel future requests or confirmed
  appointments; partners can respond and record start, completion or no-show.
  Customers cannot select a resource, and service deposits or payments are not
  collected.
- Locality search supports seller-entered neighbourhood/town text, built-in
  locality centroid geocoding, radius filtering (`radius_km`), and optional
  opt-in coordinates for distance sorting/display. There is no visual map
  display or live background location tracking.
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
  failover. Maintenance jobs use an advisory lock, so one replica performs each
  pass.
- **Password-reset email delivery is at-least-once.** A crash after SMTP
  accepts a message but before the outbox row is deleted can cause a duplicate
  email after the lease expires.
- **No distributed tracing.** The optional bearer-authenticated /metrics
  endpoint exposes bounded-cardinality HTTP metrics and Go runtime gauges.

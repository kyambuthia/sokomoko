# Roadmap

Work planned from an audit of the code on `codex/commerce-reliability`
(October 2026). Items are ordered by risk first, then value. Each item is
done only when it has a test and the full suite passes against PostgreSQL.

Status: `[x]` done, `[ ]` open. The checked items are complete; Service
directory and Later list the remaining work.

## Audit summary

Checked and found sound, so not on this list: session cookie flags and prefixes
(`__Host-` for secure host-only cookies; explicit shared-domain mode uses
`__Secure-` and trusts sibling subdomains), CSRF with constant-time comparison,
Paystack webhook HMAC with `hmac.Equal`, hashed M-Pesa callback tokens,
safe `next` redirects, session revocation on deactivation and password reset,
partner data scoping (a suspended partner's logins see and change nothing),
and index coverage for the hot queries. `govulncheck` reports nothing
reachable.

## Phase 1: correctness and security

- [x] **Refund queue for excess charges.** Late or duplicate confirmed charges
  become `payment_exceptions`. Admins request a Paystack refund (sent at most
  once) or record an M-Pesa reversal.
- [x] **Close stuck refunds by hand.** An exception left in `requested`,
  `pending`, or `review` (uncertain provider response, failed refund) had no
  action in the UI. Admins can now mark it refunded with the provider's
  reference, and the dashboard shows how many exceptions need a decision.
- [x] **Client IP behind proxies.** `ClientIP` trusted the left-most
  `X-Forwarded-For` entry, which a client controls when the proxy appends to
  the header (nginx `$proxy_add_x_forwarded_for`). It now uses the right-most
  entry, which the trusted proxy wrote.
- [x] **First-run setup through a same-host proxy.** With no
  `ADMIN_SETUP_TOKEN`, `/setup` trusted any request from loopback. A reverse
  proxy on the same machine relays internet traffic from loopback, so anyone
  could have created the root admin. Requests carrying forwarding headers no
  longer count as loopback.
- [x] **Suspended partner logins.** A suspended partner's users could still
  sign in and see an empty workspace with no explanation. Suspension now
  revokes their sessions and sign-in is refused with a clear message.
- [x] **Dead code and static analysis.** Remove what `staticcheck` reports as
  unused, and run `staticcheck` in `make lint` and CI.

## Phase 2: reliability and performance

- [x] **One query for admin metrics.** Every admin page runs five count queries
  for the header metrics. Fold them into one statement.
- [x] **Maintenance under an advisory lock.** With more than one replica,
  every process expires reservations, unpaid orders, and sessions at the same
  time. Run each pass under a PostgreSQL advisory lock so one replica does it.
- [x] **Retry password-reset email.** Delivery is one attempt with a 5-second
  timeout. Retry transient failures with bounded backoff.
- [x] **Metrics endpoint.** Optional bearer-authenticated `/metrics` exposes
  bounded-cardinality HTTP request metrics and Go runtime gauges.

## Phase 3: features

- [x] **Edit products.** Partners (and admin or staff) can change name,
  description, price, category, and stock of an existing product.
  `UpdateProduct` exists but has no caller.
- [x] **Customers cancel unpaid orders.** A customer can cancel their own order
  while it is `pending` and nothing has been captured. Stock is released.
- [x] **Change password while signed in.** Every role can change their own
  password with the current one; other sessions are revoked.
- [x] **Reactivate team members.** Deactivation is reversible by an admin.
- [x] **Rename categories and partners.**

## Phase 4: documentation

- [x] Bring `01-overview.md`, `02-architecture.md`, `03-http-surface.md`,
  `05-data-model.md`, and `10-known-limitations.md` up to date with online
  payments, store currency, refunds, and the items above, and add the missing
  `payments.md` that the admin payments page links to.

## Service directory

- [x] Seller-owned listings with draft/published/archive visibility and versioned edits.
- [x] Public browsing by service, category and locality; branded seller forms and JSON API.
- [x] Development sample listings that preserve seller edits on repeated seeds.
- [x] Service-context encrypted enquiries.
- [x] Verify transactional appointment reservations, resource availability, conflict prevention and cancellation lifecycle. The request-to-book flow, timezone and opening-hour checks, default capacity-one resources, conflict prevention, customer cancellation, partner status transitions and request expiry are implemented; verified in `internal/db/service_bookings_test.go`.
- [x] Privacy-conscious geocoding and local map discovery (backend locality geocoder, premises-only DB coordinate constraint in migration 0036, bounding-box + Haversine radius discovery, and `lat`/`lng`/`radius_km` filtering on `/services` and `/api/v1/services`; interactive map tiles remain optional/configurable).

## Later

- [x] Verify product deletion. The workspace flow is implemented and verified for ownership, cart and checkout cleanup, and concurrent add/delete behavior in `internal/db/product_delete_test.go`.
- [x] Verify product image upload. The workspace flow is implemented; add
  coverage for format, size, pixel bounds, EXIF orientation, replacement and
  removal, and active-product image serving before marking it complete.
- [x] Finish shared rate limiting verification. POST IP limits and
  authentication abuse backoff now use PostgreSQL across replicas, with a
  bounded process-local fallback on database errors. Verified with database-backed
  tests in `internal/db/rate_limit_test.go` and full `make test-isolated` pass.
- [x] Durable encrypted password-reset email outbox is implemented with transactional enqueue, replica-safe leases and retry; verified in `internal/db/password_reset_outbox_test.go`.

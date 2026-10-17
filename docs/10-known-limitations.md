# Known limitations

Gaps visible in the code as of this writing, grouped by area. Each one names
where to look.

## Commerce

- **Cash on delivery only.** `payment.SupportedMethods` returns one method. The
  card and mobile-money names exist only as labels for old records. There is no
  payment provider integration, capture, or refund flow.
- **Pricing is hard-coded.** USD, 8% estimated tax, $6.50 shipping below $80
  (`service/checkout/checkout.go`). There is no tax by region, shipping zones,
  discounts, or multi-currency support.
- **One warehouse.** The schema supports several, but every read and write uses
  the default warehouse.
- **Mixed-vendor orders.** A partner user can see and update only orders whose
  every line is theirs. Orders that mix partners can be fulfilled only by admin
  or staff.

## Catalog and vendor management

- **No admin UI for partners or product edits.** `UpsertPartner`,
  `UpdateProduct`, `DeleteProduct`, and `CreateProductImage` exist in
  `internal/db` but are called only by seed code and tests. A production
  database starts with no partners; they must be inserted with SQL.
  Categories can be created and deleted, but not renamed.
- **Partner product form has no image upload.** New products show a bundled
  fallback image.
- **No way to create partner logins.** Nothing in the app creates a user with
  the `partner` role or a `partner_users` link. Provision them with SQL.

## Identity

- No email verification, no multi-factor authentication, and no self-service
  profile or password change while signed in. `UpdateUser` and
  `UpdateUserPassword` have no callers outside tests.
- Deactivation is one-way in the UI; there is no reactivation.

## Operations

- **Single instance.** Rate limits and auth lockouts live in process memory.
  Running more than one replica weakens them and duplicates maintenance jobs.
- **Password-reset email is fire-and-forget.** It is sent from a goroutine with
  a 5-second timeout and is not retried. There is no outbox.
- **No metrics or tracing endpoint.** Only structured logs and the health
  endpoints.
- **Store calls ignore request cancellation.** Most store methods build their
  context from `context.Background()` plus the query timeout, so a disconnected
  client does not cancel its query.

## Performance

- Admin and partner lists are not paginated.

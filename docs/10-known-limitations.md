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

- **No admin UI for partners, categories, or product edits.** `UpsertPartner`,
  `CreateCategory`, `UpdateCategory`, `DeleteCategory`, `UpdateProduct`,
  `DeleteProduct`, and `CreateProductImage` exist in `internal/db` but are
  called only by seed code and tests. A production database starts with no
  partners or categories; they must be inserted with SQL.
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

- `GET /api/v1/catalog/products` loads every product, then slices the page in
  memory.
- `partner.orderBelongsToPartner` loads every fulfillment order to check
  ownership of one.
- Admin and partner lists are not paginated.

## Unused or unfinished code

- Templates not parsed by `ui.ParseTemplates`: `pages/add_product.html`,
  `pages/delivery.html`, `pages/partner_signup.html`.
- Partials `product-card`, `flash`, and `order-status` are defined but not
  called by any page.
- Web components defined but not placed in rendered pages: `ui-add-to-cart`,
  `ui-order-status`, `ui-cart-count`, `ui-cart-line`, `ui-confirm`,
  `ui-relative-time`, `ui-table-filter`, `ui-header-banner`, `ui-switch`,
  `ui-dialog`.
- `static/scripts/search.js` is not loaded and only logs to the console.
- `/api/search` has no caller in the current frontend.
- `app.CSRFSameOrigin`, `app.RateLimitByIP`, and `app.SecurityHeaders` are not
  used by the server (it uses `auth.CSRFMiddleware`, `RateLimitByKey`, and
  `SecurityHeadersWithProxy`).
- The admin `/deliveries` page renders only the shared metrics.
- `checkout.Service.Checkout` and `db.Store.PlaceOrderFromCart` (one-step
  ordering without a prepared checkout) are not used by any route.

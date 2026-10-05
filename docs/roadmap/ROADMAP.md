# Sokomoko Roadmap

Last updated: October 5, 2026

This is the single source of truth for production readiness and upcoming
features. Items are ordered by priority within each phase. Each item names the
layer or tables it touches so it can be scoped quickly.

## Where things stand

The platform runs on PostgreSQL with versioned migrations, integer-cent money,
an inventory ledger with reservation, allocation, shipment, and release states,
idempotent checkout, token CSRF protection, auth throttling, structured
logging, a distroless container image, and CI that runs the full suite under the
race detector against PostgreSQL.

### Done (October 2026 production-readiness pass)

- [x] PostgreSQL migration with an embedded, advisory-locked migration runner
- [x] Schema redesign: identity keys, `TIMESTAMPTZ`, cents, CHECK constraints, partners table, full-text search
- [x] Inventory as the single source of truth; stock released on cancel and decremented on dispatch
- [x] Row-locked, deadlock-safe stock reservation and allocation with retry on serialization failures
- [x] Cart changes invalidate open checkouts and their stock holds
- [x] Idempotent checkout submission keyed by checkout token
- [x] Token CSRF protection on every state-changing form, plus JSON clients via `X-CSRF-Token`
- [x] Auth throttling, proxy-aware client IP (`TRUST_PROXY_HEADERS`), session rotation on login, timing-safe unknown-user logins
- [x] bcrypt without truncation (72-byte limit enforced); salt column removed
- [x] Post/redirect/get with one-time flash cookies instead of query-string messages
- [x] Startup config validation; production refuses to boot without `DATABASE_URL`, `ALLOWED_HOSTS`, `PASSWORD_RESET_BASE_URL`
- [x] Structured `slog` logs (JSON in production) with validated request IDs
- [x] Background expiry of checkout stock holds
- [x] Dockerfile (static, non-root, distroless), docker-compose, Makefile, `.env.example`
- [x] CI: gofmt, `go mod tidy` check, vet, JS syntax, `go test -race` on PostgreSQL, image build
- [x] Web components for live cart, add-to-cart, checkout countdown, order tracking, table filters and more

## Phase 0: Before the first real customer

These block a public launch.

1. **Real payments.** Replace `card_placeholder` and `mobile_money_placeholder`
   with a provider such as Stripe, or M-Pesa via Daraja for the local market.
   Add a payment intent flow, signed webhook handling (`/webhooks/payments`
   outside CSRF, verified by signature), and statuses `authorized`, `captured`,
   `failed`, `refunded`. Tables: `payments`, `payment_attempts`; new
   `payment_events` for webhook idempotency.
2. **Transactional admin bootstrap.** `AdminSetup` creates the admin and then
   staff accounts one by one. A failure midway leaves a partial state, and two
   concurrent setups can both pass the "no admin yet" check. Wrap it in one
   transaction guarded by `pg_advisory_xact_lock`. Layer: `auth`, `db`.
3. **Backups and restore runbook.** Daily `pg_dump` or managed point-in-time
   recovery, restore drills, and documented RPO and RTO.
4. **Secrets handling.** Load SMTP and payment secrets from the platform secret
   store; never from committed files. Document rotation.
5. **TLS and proxy deployment guide.** A reference reverse proxy config (Caddy or
   nginx) with HSTS, `TRUST_PROXY_HEADERS=true`, `SESSION_COOKIE_DOMAIN`, and
   host mapping for the three surfaces.
6. **Legal pages.** Terms, privacy, and returns pages, and real footer links
   (the footer links are currently `#`).

## Phase 1: Commerce essentials

1. **Customer order detail and cancellation.** `/account/orders/{id}` with line
   items and a timeline from `audit_logs`. Customers can cancel while
   `partner_status = new`, which releases allocated stock through the existing
   inventory transition.
2. **Shipping rates.** Replace the flat $6.50 / free-over-$80 rule with shipping
   profiles per region and a weight or price matrix. New `shipping_zones` and
   `shipping_rates`; `checkout.CalculateSummary` takes an address.
3. **Real tax calculation.** Replace the estimated 8% with configurable tax rates
   per region, or a provider. Store the tax breakdown per order.
4. **Saved addresses.** `addresses` table, a picker at checkout, and an address
   on the order snapshot instead of free text.
5. **Product management UI.** Edit, archive, image upload (object storage plus
   signed URLs), stock adjustments with a reason, and a stock movement history
   view backed by `stock_movements`.
6. **Pagination and sorting** for the storefront, search, admin orders, and
   products. `db.ProductFilter` already supports `Limit` and `Offset`; add
   keyset pagination for orders.
7. **Transactional email.** Order confirmation, dispatch, and delivery emails
   through the existing `notify` SMTP sender, sent from an outbox table
   (`email_outbox`) so a mail failure never rolls back an order.

## Phase 2: Multi-vendor marketplace

The schema already records `partner_id` on products and order items.

1. **Partner accounts.** Let partner users sign in to the partner host and see
   only their own products and order lines. Add a `partner_members(partner_id,
   user_id, role)` table and a `partner` role scope in `auth`.
2. **Per-partner fulfillment.** Split orders into `fulfillments` (one per partner
   per order) with their own status, tracking number, and inventory state, so
   one vendor dispatching does not ship another vendor's lines.
3. **Payouts and commission.** `partner_payouts`, a commission rate per partner,
   and a ledger reconciled against captured payments.
4. **Multi-warehouse sourcing.** Choose a warehouse per line (nearest, or most
   stock) instead of always the default. `inventory_stocks` is already keyed by
   `(product_id, warehouse_id)`.
5. **Partner analytics.** Sales, top products, and fulfillment SLA per partner.

## Phase 3: Discovery and conversion

1. **Product variants** (size, colour): a `product_variants` table with its own
   SKU, price, and stock, with cart and order lines pointing at variants.
2. **Category landing pages** with SEO-friendly URLs (`/c/{slug}`), breadcrumbs,
   and `sitemap.xml`.
3. **Ratings and reviews** from verified purchasers, with moderation in admin.
4. **Wishlist** (the old "Saved" header link is gone until this exists).
5. **Promotions:** coupon codes, percentage or fixed discounts, and sale prices
   (`ui-price` already supports an `original` price).
6. **Search improvements:** typo tolerance with `pg_trgm`, facets (price,
   category, partner), and in-stock filtering in the UI.
7. **Guest checkout** with an email-verified order lookup.

## Phase 4: Identity and trust

1. Email verification for new accounts.
2. TOTP MFA for admin and staff, required in production.
3. Session management page: list active sessions and revoke them.
4. Account deletion and data export.
5. Audit-log export and a retention policy.

## Phase 5: Operability and scale

1. **Metrics.** A Prometheus `/metrics` endpoint, or OpenTelemetry, covering
   request rate, latency, error rate, checkout outcomes, auth failures,
   reservation expiries, and DB pool saturation.
2. **Tracing.** OpenTelemetry spans across the routes, service, and db layers.
   Thread `context.Context` from requests into services and the store; store
   calls currently use bounded internal contexts.
3. **Shared rate-limit and abuse state** (Redis or PostgreSQL) so limits hold
   across replicas. Today they are per process.
4. **Background job runner** with leader election (an advisory lock) so the
   cleanup and expiry jobs run once per cluster rather than once per replica.
   They are safe to run concurrently today, just redundant.
5. **Load testing** with k6 scripts for browse, search, and checkout, plus SLOs
   (for example p95 under 300 ms for catalog pages).
6. **Supply chain:** `govulncheck` and dependency review in CI, SBOM generation,
   and image signing.
7. **Read replicas** for catalog and search queries once traffic warrants.

## Phase 6: UX polish

1. Accessibility audit: keyboard paths, focus management in `ui-dialog`, colour
   contrast in both themes, and form error association (`aria-describedby`).
2. Image pipeline: responsive `srcset`, WebP or AVIF, and lazy loading
   everywhere.
3. An offline-friendly service worker for static assets (the manifest is
   already in place).
4. Internationalisation: currency per store (prices are stored in cents with a
   currency column) and translated templates.

## Known limitations

- Fulfillment status is tracked per order, not per partner (see Phase 2).
- All stock is sourced from the default warehouse.
- Rate limits and auth throttling are in memory, so each replica limits independently.
- The admin "Deliveries" page is a placeholder.
- Payment methods other than cash on delivery are placeholders that record a captured payment without charging anyone.

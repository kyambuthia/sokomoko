# Overview

Sokomoko is a multi-vendor marketplace. Partners (vendors) list products,
customers buy them, and the platform team runs operations. Everything runs in
one Go process backed by one PostgreSQL database.

## Workspaces

The process routes each request by its `Host` header (see
[Architecture](02-architecture.md#host-routing)).

### Storefront (`localhost`, or your shop domain)

- Home page with the catalog and category navigation
- Product detail pages at `/products/<slug>`
- Search with full-text matching and category filter
- Customer signup, login, logout, password reset
- Cart: add, change quantity, remove
- Checkout: stock is held for 15 minutes while the customer enters a delivery
  address; placing the order is idempotent
- Account page with order history and a live fulfillment tracker

### Admin workspace (`admin.` host)

- First-run `/setup` that creates the root admin and optional staff accounts
- Dashboard metrics: product count, users by role, active sessions
- Product list (read-only)
- Order list with full status editing (admin only)
- Sales report: revenue and order counts by status
- Deliveries board: open orders counted by delivery stage and listed oldest first
- Categories: create and delete (admin and staff)
- Partners: onboard vendors, create partner logins, suspend or reactivate (admin only). A suspended partner's products disappear from the storefront and carts.
- Team list with user deactivation (admin only)
- Audit log (admin only)
- Staff account creation (admin only)

### Partner workspace (`partner.` host)

- Store settings (name, slug, description, contact email)
- Dashboard with product count and fulfillment summary
- Product list and new-product form
- Order fulfillment: move orders through partner and delivery statuses

Admins and staff see every product and order here. A user with the `partner`
role sees only the products of the partner they are linked to, and only orders
whose every line belongs to that partner.

## Roles

| Role | Signs in at | Can do |
| --- | --- | --- |
| `user` | Storefront `/login` | Shop, check out, view own orders |
| `staff` | Admin or partner `/login` | Admin read views, partner workspace, fulfillment |
| `admin` | Admin or partner `/login` | Everything staff can, plus order overrides, team, audit, staff signup |
| `partner` | Partner `/login` only | Partner workspace limited to their own partner's data |

Each login page accepts only its own roles. A customer cannot sign in on the
admin host, and an admin cannot sign in on the storefront.

## Commerce rules in code

These are hard-coded in `internal/service/checkout/checkout.go`:

- Currency: USD
- Shipping: $6.50 flat, free at a subtotal of $80 or more
- Tax: estimated 8% of the subtotal
- Payment: cash on delivery only. A payment row is recorded as `pending`
  with provider `manual_cash_on_delivery`. No online payment provider exists.

## Demo data

`sokomoko seed` and `serve --seed` load four demo partners and a small demo
catalog with images in `internal/ui/static/images/products/`. They also create
an admin when `ADMIN_PASSWORD` is set. Seeding is refused when `ENV=production`.

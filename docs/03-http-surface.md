# HTTP surface

All routes are registered in `internal/routes/register.go` (HTML) and
`internal/api/handler.go` (JSON). Every host also serves:

| Path | Purpose |
| --- | --- |
| `GET/HEAD /healthz` | Process liveness. Always `200 ok`. |
| `GET/HEAD /readyz` | Database ping with a 2 s deadline. `503` while draining or when the database is unreachable. |
| `/static/...` | Embedded assets. In development, files under `internal/ui/static/` on disk take precedence. |

Workspace pages link to other hosts through the `/storefront` and `/catalog`
redirect routes listed below. The target is derived from the current host by swapping the `admin.` or `partner.` prefix
(`routes/hosts.go`).

Access column legend: **public** (no session needed), **signed in** (any
session, otherwise redirect to `/login?next=...`), or a list of roles.

## Storefront host

| Path | Methods | Access | Handler | Notes |
| --- | --- | --- | --- | --- |
| `/` | GET | public | `Root` | Home page and catalog |
| `/products/<slug>` | GET | public | `ProductDetail` | 404 for unknown slugs |
| `/search` | GET | public | `Search` | `q`, `category` |
| `/api/categories` | GET | public | `CategoriesAPI` | JSON `{categories}` for the header category nav |
| `/login` | GET, POST | public | `auth.Login` | `user` role only; honours a safe `next` |
| `/signup` | GET, POST | public | `auth.SignUp` | Creates a `user` and signs in |
| `/logout` | POST | public | `auth.Logout` | Revokes the session |
| `/password-reset/request` | GET, POST | public | `auth.PasswordResetRequest` | `user` accounts only |
| `/password-reset/confirm` | GET, POST | public | `auth.PasswordResetConfirm` | `token` query or form field |
| `/cart` | GET | signed in | `CartPage` | |
| `/cart/add` | POST | signed in | `CartAdd` | `product_id`, `quantity` |
| `/cart/update` | POST | signed in | `CartUpdate` | Quantity 0 removes the line |
| `/cart/remove` | POST | signed in | `CartRemove` | |
| `/checkout` | GET, POST | signed in | `Checkout` | See below |
| `/account` | GET | signed in | `Account` | Order history |

`/api/categories` is the one unversioned JSON route left. New client code
should use `/api/v1`.

Cart actions redirect back with a flash message. When the request has
`Accept: application/json` they return the updated cart as JSON instead.

**Checkout flow.** `GET /checkout?checkout=<token>` resumes or creates an open
checkout: it reserves stock for the whole cart for 15 minutes and snapshots
prices. `POST /checkout` sends `delivery_address`, `payment_method`, and
`idempotency_key` (the checkout token). On success it redirects to
`/account#order-<id>` with a flash message. On failure it saves the typed
address as a draft and re-renders the form with `422`.

## Admin host

| Path | Methods | Access | Handler |
| --- | --- | --- | --- |
| `/setup` | GET, POST | setup token or loopback, only while no admin exists | `auth.AdminSetup` |
| `/login` | GET, POST | public; redirects to `/setup` until an admin exists | `auth.AdminLogin` (admin, staff) |
| `/logout` | POST | public | `auth.Logout` |
| `/password-reset/request`, `/password-reset/confirm` | GET, POST | public | admin and staff accounts |
| `/staff/signup` | GET, POST | admin | `auth.StaffSignUp` |
| `/storefront` | GET | public | Redirect to the storefront host |
| `/catalog`, `/catalog/new` | GET | public | Redirect to the partner host's `/products` and `/products/new` |
| `/` | GET | admin, staff | `AdminDashboard` |
| `/products` | GET | admin, staff | `AdminProducts` |
| `/orders` | GET, POST | admin, staff (POST needs admin) | `AdminOrders` |
| `/reports` | GET | admin, staff | `AdminReports` |
| `/deliveries` | GET | admin, staff | `AdminDeliveries` (delivery board) |
| `/team` | GET, POST | admin | `AdminTeam` (POST deactivates a user) |
| `/audit` | GET | admin | `AdminAudit` |

## Partner host

| Path | Methods | Access | Handler |
| --- | --- | --- | --- |
| `/login` | GET, POST | public | `auth.WorkspaceLogin` (admin, staff, partner) |
| `/storefront` | GET | public | Redirect to the storefront host |
| `/logout` | POST | public | `auth.Logout` |
| `/password-reset/request`, `/password-reset/confirm` | GET, POST | public | admin, staff, partner accounts |
| `/signup` | GET, POST | admin | `auth.StaffSignUp` (creates staff) |
| `/` | GET | admin, staff, partner | `PartnerRoot`: redirects to `/setup` or `/dashboard` |
| `/setup` | GET, POST | admin, staff, partner (POST needs admin or staff) | `PartnerSetup` (store settings; read-only for partners) |
| `/dashboard` | GET | admin, staff, partner | `PartnerDashboard` |
| `/products` | GET | admin, staff, partner | `PartnerProducts` |
| `/products/new` | GET, POST | admin, staff, partner | `PartnerProductNew` |
| `/orders` | GET, POST | admin, staff, partner | `PartnerOrders` (`status` filter; POST updates fulfillment) |

Before any admin exists, the partner login page returns `503` with a message to
finish setup on the admin host.

## JSON API (`/api/v1`)

Served on every allowed host. Uses the same session cookie as the HTML pages;
there are no API keys or bearer tokens.

### Conventions

- Success: `{"data": ...}`. Error: `{"error": {"code": "...", "message": "..."}}`.
- Request bodies are JSON, at most 1 MiB, one object, unknown fields rejected.
- Unsafe methods need the CSRF token in `X-CSRF-Token` (the pages expose it in
  `<meta name="csrf-token">`).
- Unauthenticated calls get `401 authentication_required`, never a redirect.
- Responses are `Cache-Control: no-store`, except public catalog reads
  (`public, max-age=30`, search `max-age=15`).
- Lists that paginate take `page` (from 1) and `limit` (default 20, max 50).

### Endpoints

| Method and path | Access | Purpose |
| --- | --- | --- |
| `GET /catalog/products` | public | Paginated product list: `{items, page, limit, total}` |
| `GET /catalog/products/<slug>` | public | One product |
| `GET /catalog/search?q=&category=` | public | Search results |
| `GET /me` | signed in | Current user |
| `GET /cart` | signed in | Cart lines and subtotal |
| `POST /cart/items` | signed in | Add `{product_id, quantity}` |
| `PATCH /cart/items/<product_id>` | signed in | Set `{quantity}` |
| `DELETE /cart/items/<product_id>` | signed in | Remove a line |
| `POST /checkout` | signed in | Resume or prepare a checkout and hold stock. Optional body `{token}`. Returns `201` with the checkout and its `token` |
| `GET /checkout?token=` | signed in | Read a checkout (re-prepares it if it is no longer open) |
| `GET /checkout/<token>` | signed in | Read a checkout |
| `PATCH /checkout/<token>` | signed in | Save `{delivery_address, payment_method}` |
| `POST /checkout/<token>/place-order` | signed in | Place the order. Optional `Idempotency-Key` header must equal the token. Replays return the original order. |
| `GET /account/orders` | signed in | Own orders |
| `GET /account/orders/<id>` | signed in | One own order |
| `GET /admin` or `/admin/metrics` | admin, staff | Dashboard metrics |
| `GET /admin/products` | admin, staff | Product list |
| `GET /admin/orders` | admin, staff | All orders |
| `PATCH /admin/orders/<id>` | admin | Set `{status, partner_status, delivery_status, delivery_notice}` |
| `GET /admin/reports` | admin, staff | Sales report |
| `GET /admin/team` | admin | Team members |
| `DELETE /admin/team/<id>` | admin | Deactivate a user |
| `GET /admin/audit` | admin | Audit log |
| `GET /partner/dashboard` | admin, staff, partner | Dashboard data |
| `GET /partner/products` | admin, staff, partner | Products and categories |
| `POST /partner/products` | admin, staff, partner | Create a product |
| `GET /partner/orders?status=` | admin, staff, partner | Orders and summary |
| `PATCH /partner/orders/<id>` | admin, staff, partner | Update `{partner_status, delivery_status, delivery_notice}` |
| `GET /partner/settings` | admin, staff, partner | Store settings (`404 store_not_configured` if unset) |
| `PATCH /partner/settings` | admin, staff | Save store settings |

Partner-role callers are scoped exactly as in the partner HTML workspace.
Money fields are JSON numbers in dollars (for example `12.5`), converted from
integer cents at the edge.

# HTTP surface

All routes are registered in `internal/routes/register.go` (HTML) and
`internal/api/handler.go` (JSON). Every host also serves:

| Path | Purpose |
| --- | --- |
| `GET/HEAD /healthz` | Process liveness. Always `200 ok`. |
| `GET/HEAD /readyz` | Database ping with a 2 s deadline. `503` while draining or when the database is unreachable. |
| `/static/...` | Embedded assets. In development, files under `internal/ui/static/` on disk take precedence. |

Workspace pages link to other hosts through the `/storefront`, `/catalog`, and
`/courier` redirect routes listed below. The target is derived from the current
host by removing an existing `admin.`, `partner.`, or `courier.` prefix and
adding the requested prefix (`routes/hosts.go`).

Access column legend: **public** (no session needed), **signed in** (any
session, otherwise redirect to `/login?next=...`), or a list of roles.

## Storefront host

| Path | Methods | Access | Handler | Notes |
| --- | --- | --- | --- | --- |
| `/` | GET | public | `Root` | Home page and catalog |
| `/products/<slug>` | GET | public | `ProductDetail` | 404 for unknown slugs |
| `/search` | GET | public | `Search` | `q`, `category` |
| `/services` | GET, HEAD | public | `Services` | Literal `q`, `category`, `area`, `delivery_mode`, `partner_id`, `page`; 24 per page |
| `/services/<slug>` | GET, HEAD | public | `ServiceDetail` | Published listings of active shops only |
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
| `/account` | GET | signed in | `Account` | Order history, newest first, 20 per page (`?page=`) |
| `/account/orders/<id>/cancel` | POST | signed in | `AccountOrderCancel` | Own order, while `pending`, partner status `new`, and unpaid |
| `/account/password` | GET, POST | signed in | `auth.PasswordChange` | Needs the current password; revokes every session and issues a new one |
| `/payments/order/<id>` | GET, POST | signed in | `PaymentPage` | Pay or retry an online payment for an own order |
| `/payments/return` | GET | signed in | `PaymentReturn` | Where Paystack sends the customer back |
| `/payments/webhooks/<provider>` | POST | provider (signed) | `PaymentWebhook` | Paystack events, HMAC-SHA512 |
| `/payments/callback/<provider>/<token>` | POST | provider (token) | `PaymentCallback` | M-Pesa STK results |

Authenticated messaging routes on the storefront, partner and courier hosts,
their JSON counterparts, and the admin report queue are documented in
[Messaging HTTP contracts](messaging.md#http-contracts). New encrypted device
operations use `POST /api/v1/encrypted-chat`; see
[encrypted messaging](encrypted-messaging.md) for its signed envelope and
authorization contract. Plaintext send endpoints reject new sends.

`/api/categories` is the one unversioned JSON route left. New client code
should use `/api/v1`.

Cart actions redirect back with a flash message. When the request has
`Accept: application/json` they return the updated cart as JSON instead. The
JavaScript storefront serializes cart reads and writes within the current page,
including badge initialization, and announces each successful snapshot before
starting the next request. Failed requests release the queue. Pending line
removal blocks later quantity autosubmits; detached quantity controls cancel
their pending timer. Cart quantity is required: clearing it is an unfinished
edit and does not autosubmit a removal. Only nonempty values satisfying the
input constraints autosubmit; an explicit zero still removes the line. This
does not coordinate separate browser tabs.

Customer order progress refreshes every 30 seconds while the page is visible.
Only one request per widget is in flight; responses from a previous connection
are ignored. Polling stops for cancelled orders and completed fulfillment. An
overall `delivered` order status also marks progress complete, even if partner
or delivery fields still show an earlier stage.

Payment status polling allows one request per widget and stops after five
minutes with a manual-refresh message. Pending responses from detached widgets
or a previous connection are ignored. Only `paid`, `failed`, `cancelled` and
`offline` responses trigger a reload; pending or unknown states keep the page
visible. An expired polling window never treats a late response as permission
to reload.

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
| `/couriers/new` | GET, POST | admin | `auth.CourierSignUp` (create a courier account with username and initial password; no email) |
| `/courier` | GET | public | Redirect to the courier host's `/login` |
| `/categories` | GET, POST | admin, staff | `AdminCategories` (list, create, rename with `action=update`, delete); `?edit=<id>` opens the edit form |
| `/team` | GET, POST | admin | `AdminTeam` (GET lists 50 accounts per page, `?q=` searches username or email; POST deactivates a user, or reactivates one with `action=reactivate`) |
| `/partners` | GET, POST | admin | `AdminPartners` (create, rename with `action=update`, suspend or reactivate, create partner logins); `?edit=<id>` opens the edit form |
| `/payments` | GET, POST | admin | `AdminPayments` (pricing, providers, refund queue: `refund`, `refund_refresh`, `refund_resolve`) |
| `/password` | GET, POST | admin, staff | `auth.PasswordChange` |
| `/audit` | GET | admin | `AdminAudit` |

## Partner host

| Path | Methods | Access | Handler |
| --- | --- | --- | --- |
| `/login` | GET, POST | public | `auth.WorkspaceLogin` (admin, staff, partner) |
| `/storefront` | GET | public | Redirect to the storefront host |
| `/logout` | POST | public | `auth.Logout` |
| `/password-reset/request`, `/password-reset/confirm` | GET, POST | public | admin, staff, partner accounts |
| `/` | GET | admin, staff, partner | `PartnerRoot`: redirects to `/setup` or `/dashboard` |
| `/setup` | GET, POST | admin, staff, partner (POST needs admin or staff) | `PartnerSetup` (store settings; read-only for partners) |
| `/dashboard` | GET | admin, staff, partner | `PartnerDashboard` |
| `/products` | GET | admin, staff, partner | `PartnerProducts` |
| `/products/new` | GET, POST | admin, staff, partner | `PartnerProductNew` |
| `/products/<id>/edit` | GET, POST | admin, staff, partner (own products only; others get 404) | `PartnerProductEdit` |
| `/password` | GET, POST | admin, staff, partner | `auth.PasswordChange` |
| `/orders` | GET, POST | admin, staff, partner | `PartnerOrders` (`status` filter; POST updates fulfillment) |

Seller-only service pages on the partner host are `GET /services`,
`GET|POST /services/new`, `GET|POST /services/<id>/edit` and
`GET|HEAD /services/<id>/preview` (private saved listing preview). Admin and staff
accounts cannot manage these offerings. POST requires CSRF, all editable fields
and a version number (`0` for creation). Validation errors preserve the entered
text; a stale edit returns 409 without overwriting the current record.

Partners control order preparation through `dispatched`. Dispatching ships the
order and starts the courier handoff. Courier accounts update delivery stages
after that point; partner forms cannot mark a courier delivery complete.

Before any admin exists, the partner login page returns `503` with a message to
finish setup on the admin host.

## Courier host

| Path | Methods | Access | Handler | Notes |
| --- | --- | --- | --- | --- |
| `/login` | GET, POST | public | `auth.CourierLogin` | Courier role only |
| `/logout` | POST | public | `auth.Logout` | Revokes the session |
| `/storefront` | GET | public | Redirect to the storefront host | |
| `/password` | GET, POST | courier | `auth.PasswordChange` | Requires the current password |
| `/` | GET, HEAD | public | Redirect to `/deliveries` | |
| `/deliveries` | GET | courier | `CourierDeliveries` | Own assignments and delivered history; optional `view=all|active|completed`; includes recipient handle and item quantities; address is hidden after completion |
| `/deliveries/<id>` | POST | courier | `CourierDeliveryUpdate` | CSRF protected; accepts `status` and a fixed `note` reason for failed attempts |

Couriers cannot request password-reset email. An admin provisions their
username and initial password, which should be shared out of band. A courier
cannot reassign an order or view another courier's jobs.

## JSON API (`/api/v1`)

Served under `/api/v1` on the storefront, admin, partner, and courier hosts.
The docs host serves the public guide only. The API uses the same session
cookie as the HTML pages; there are no API keys or bearer tokens.

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
| `GET /catalog/services?q=&category=&area=&delivery_mode=&partner_id=` | public | Published services; `{items, page, limit, total}`; limit 1–50, page 1–1000 |
| `GET /catalog/services/<slug>` | public | Published service details; 404 for private, archived or suspended-shop listings |
| `GET /partner/services` | partner | Own shop listings including drafts; paginated |
| `POST /partner/services` | partner | Create listing, version 0; owner derived from session |
| `GET /partner/services/<id>` | partner | Own editable listing; another shop returns 404 |
| `PATCH /partner/services/<id>` | partner | Full editable field replacement plus current version; stale version returns 409 |
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
| `GET /account/orders/<id>` | signed in | One own order, with `cancellable` |
| `GET /account/orders/<id>/payment` | signed in | Payment state of an own order (`pending`, `paid`, `failed`, `cancelled`, `offline`); polling it also confirms a pending payment with the provider |
| `POST /account/orders/<id>/cancel` | signed in | Cancel an own new, unpaid order. `409 order_not_cancellable` once accepted or paid. Returns the order |
| `GET /admin` or `/admin/metrics` | admin, staff | Dashboard metrics |
| `GET /admin/products` | admin, staff | Product list |
| `GET /admin/orders` | admin, staff | All orders |
| `PATCH /admin/orders/<id>` | admin | Set `{status, partner_status, delivery_status, delivery_notice}` |
| `GET /admin/reports` | admin, staff | Sales report |
| `GET /admin/team` | admin | Team members: `{items, page, limit, total}`; `q` searches username or email |
| `DELETE /admin/team/<id>` | admin | Deactivate a user |
| `GET /admin/audit` | admin | Audit log |
| `GET /partner/dashboard` | admin, staff, partner | Dashboard data |
| `GET /partner/products` | admin, staff, partner | Products and categories |
| `POST /partner/products` | admin, staff, partner | Create a product |
| `PATCH /partner/products/<id>` | admin, staff, partner | Change any of `{name, description, price, stock, category_id}`; omitted fields keep their values. `stock` sets available stock |
| `GET /partner/orders?status=` | admin, staff, partner | Orders and summary |
| `PATCH /partner/orders/<id>` | admin, staff, partner | Update `{partner_status, delivery_status, delivery_notice}` |
| `GET /admin/deliveries` | admin, staff | Shipped orders awaiting delivery and active courier accounts with `active_deliveries` and `failed_deliveries` counts; excludes prices and payment data |
| `POST /admin/deliveries/assign` | admin, staff | Assign `{order_id, courier_id}`; only shipped orders can be assigned, and picked-up deliveries cannot be reassigned |
| `GET /courier/deliveries` | courier | Own delivery assignments and delivered history; optional `view=all|active|completed`; completed `delivery_address` is empty |
| `PATCH /courier/deliveries/<id>` | courier | Update `{status, note}` through `picked_up`, `out_for_delivery`, `delivered`, or `delivery_failed` |
| `GET /partner/settings` | admin, staff, partner | Store settings (`404 store_not_configured` if unset) |
| `PATCH /partner/settings` | admin, staff | Save store settings |

When `status` is `delivery_failed`, `note` must be one of `Recipient
unavailable`, `Address issue`, `Item damaged`, `Recipient declined`, or
`Other`. Free-form failure notes are not retained. Partner update requests keep
the legacy `delivery_status` field for compatibility, but the service ignores
it; only the courier workflow advances post-dispatch delivery states.

Partner-role callers are scoped exactly as in the partner HTML workspace.
Money responses retain the existing decimal JSON numbers for v1 clients and
also include exact integer minor-unit fields plus a three-letter `currency`.
For example, a product priced at KES 12.50 has `"price": 12.5`,
`"price_minor": 1250`, and `"currency": "KES"`. Cart, checkout, order,
and payment totals follow the same `*_minor` convention. Clients should use
the integer fields for arithmetic; all supported store currencies currently
have two decimal places.

Courier update retries for the same current action state and failure category
succeed without duplicate timeline events. An old state submitted after a later
handoff, a changed failure category, a cancelled order, or a request by a former
courier still fails under the normal transition/authorization checks.

Service request examples, validation rules, pricing and limitations are documented
in [Services and local discovery](services-and-local-discovery.md#http-contract).

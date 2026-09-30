# Dynamic JSON API

Sokomoko exposes a same-origin JSON API for progressive enhancement. Go
templates remain the canonical initial render and no-JavaScript fallback; the
API updates small regions of those pages after they load.

The API is served by the same Go binary under `/api/v1`. It shares the service
and database layers with HTML routes and does not contain independent business
rules.

## Response shape

Successful responses use a `data` envelope:

    {"data":{"id":1,"name":"Example"}}

Errors use stable codes and human-readable messages:

    {"error":{"code":"insufficient_stock","message":"Requested quantity is unavailable"}}

API handlers return JSON errors rather than HTML redirects. The shared
middleware also returns an `X-Request-Id` response header and includes that ID
in server logs.

## Authentication

The dynamic browser client uses the existing HttpOnly session cookie with
`credentials: same-origin`. Unsafe API requests must send the current session
CSRF token in `X-CSRF-Token`; the shared client reads it from the page meta
tag.

An unauthenticated API request receives `401` JSON. It is not redirected to a
login page. The HTML forms remain available as the fallback path.

## Catalog

| Method | Path | Access | Purpose |
| --- | --- | --- | --- |
| GET | `/api/v1/catalog/products` | public | Paginated product list |
| GET | `/api/v1/catalog/products/{slug}` | public | Product detail |
| GET | `/api/v1/catalog/search?q=...` | public | Search/autocomplete results |

Catalog reads use short public cache lifetimes. `limit` is capped at 50.

## Customer commerce

| Method | Path | Access | Purpose |
| --- | --- | --- | --- |
| GET | `/api/v1/me` | customer/session | Current user |
| GET | `/api/v1/cart` | customer | Current cart |
| POST | `/api/v1/cart/items` | customer | Add an item |
| PATCH | `/api/v1/cart/items/{product_id}` | customer | Change quantity |
| DELETE | `/api/v1/cart/items/{product_id}` | customer | Remove an item |
| POST | `/api/v1/checkout` | customer | Prepare a checkout reservation |
| GET | `/api/v1/checkout/{token}` | customer | Read checkout state |
| PATCH | `/api/v1/checkout/{token}` | customer | Save address/payment draft |
| POST | `/api/v1/checkout/{token}/place-order` | customer | Place the order |
| GET | `/api/v1/account/orders` | customer | Order history |
| GET | `/api/v1/account/orders/{id}` | customer | Order detail |

Checkout preparation is a `POST` because it reserves stock. Order placement
uses the opaque checkout token as the idempotency key and accepts the same
value in the `Idempotency-Key` header.

## Workspace APIs

Admin and partner endpoints reuse the current role guards. Partner users are
scoped to their own catalog and fulfillment records; admin and staff users
retain global operational access.

Workspace routes include:

- `/api/v1/admin/metrics`
- `/api/v1/admin/products`
- `/api/v1/admin/orders`
- `/api/v1/admin/reports`
- `/api/v1/admin/team`
- `/api/v1/admin/audit`
- `/api/v1/partner/dashboard`
- `/api/v1/partner/products`
- `/api/v1/partner/orders`
- `/api/v1/partner/settings`

## Browser behavior

The shared client is in `internal/ui/static/scripts/api.js`; dynamic behavior
is in `internal/ui/static/scripts/dynamic.js`. Current enhancements cover:

- catalog autocomplete
- add-to-cart, cart updates, and removal
- checkout placement
- account order-status refresh

If an API request fails, the page shows a toast where possible. The original
HTML form is preserved for navigation and fallback behavior.

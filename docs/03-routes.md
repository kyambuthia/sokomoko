# Routes

Routes are registered in
[internal/routes/register.go](../internal/routes/register.go). The host router
rejects hosts that are not in ALLOWED_HOSTS before selecting a mux.

## Public host

Host: localhost, 127.0.0.1, or another non-admin/non-partner allowed host.

| Method | Path | Access | Purpose |
| --- | --- | --- | --- |
| GET | /healthz | public | Liveness |
| GET | /readyz | public | Database readiness |
| GET | / | public | Storefront |
| GET | /products/{slug} | public | Product detail |
| GET | /search | public | Product search |
| GET, POST | /login | public | Customer login |
| GET, POST | /signup | public | Customer signup |
| GET, POST | /password-reset/request | public | Request customer reset |
| GET, POST | /password-reset/confirm | public | Confirm customer reset |
| POST | /logout | session-aware | Delete the current session |
| GET | /cart | user | View cart |
| POST | /cart/add | user | Add a product |
| POST | /cart/update | user | Change quantity |
| POST | /cart/remove | user | Remove a product |
| GET, POST | /checkout | user | Prepare and place checkout |
| GET | /account | user | Account and order history |
| GET | /static/... | public | Static assets |

Customer pages use role user. Cart, checkout, and account routes are wrapped
with AuthMiddleware.

## Admin host

Host: admin.localhost or another allowed host beginning with admin.

| Method | Path | Access | Purpose |
| --- | --- | --- | --- |
| GET | /healthz | public | Liveness |
| GET | /readyz | public | Database readiness |
| GET, POST | /setup | conditional | First admin bootstrap |
| GET, POST | /login | public | Admin/staff login |
| GET, POST | /password-reset/request | public | Admin/staff reset request |
| GET, POST | /password-reset/confirm | public | Admin/staff reset confirmation |
| GET, POST | /staff/signup | admin | Create staff account |
| GET | / | admin, staff | Dashboard |
| GET | /products | admin, staff | Product view |
| GET, POST | /orders | admin, staff | Orders and admin order updates |
| GET | /reports | admin, staff | Reports |
| GET | /deliveries | admin, staff | Delivery view |
| GET | /audit | admin | Audit log |
| GET, POST | /team | admin | Team and user deactivation |
| GET | /static/... | public | Static assets |

Staff can reach the general operational pages, while audit, team actions, and
staff creation require role admin. The admin service also restricts which
order mutations staff can perform.

## Partner host

Host: partner.localhost or another allowed host beginning with partner.

| Method | Path | Access | Purpose |
| --- | --- | --- | --- |
| GET | /healthz | public | Liveness |
| GET | /readyz | public | Database readiness |
| GET | / | admin, staff | Partner landing page |
| GET, POST | /login | public | Reused admin/staff login |
| GET, POST | /signup | admin | Legacy staff-creation path |
| GET, POST | /password-reset/request | public | Admin/staff reset request |
| GET, POST | /password-reset/confirm | public | Admin/staff reset confirmation |
| GET, POST | /setup | admin, staff | Store settings |
| GET | /dashboard | admin, staff | Partner metrics |
| GET | /products | admin, staff | Product list |
| GET, POST | /products/new | admin, staff | Create product |
| GET, POST | /orders | admin, staff | Fulfillment and delivery updates |
| GET | /static/... | public | Static assets |

Despite the host name and reset-page wording, the partner mux currently
accepts admin and staff users, not users with role partner. Product and order
operations are not currently partitioned by partner account.

## Cross-cutting behavior

Unsafe requests carrying a session cookie must include the session CSRF token as
a csrf_token form field or X-CSRF-Token header. See
[authentication](04-authentication.md) and [known issues](08-known-issues.md) for templates
that currently fail to provide one.

Routes explicitly reject unsupported HTTP methods with 405. Unknown paths fall
through to the route-specific not-found behavior.

# Authentication

Authentication is implemented in
[internal/auth/auth.go](../internal/auth/auth.go) and registered in
[internal/routes/register.go](../internal/routes/register.go).

## Account classes

| Role | Created by | Accepted by |
| --- | --- | --- |
| user | customer signup | customer login and customer reset |
| staff | admin/staff signup | admin login and operational reset |
| admin | admin setup or admin seed | admin login and operational reset |
| partner | seed workflow | partner host login and partner operations |

The partner host is an operational surface for admin, staff, and partner users.
Partner users are scoped to their own catalog and order operations.

## Sessions

Successful login creates:

- a random opaque session_token
- a separate random CSRF token
- a 24-hour database session

The cookie is HttpOnly, SameSite=Lax, and Secure for HTTPS requests or when
ENV=production. The optional SESSION_COOKIE_DOMAIN is applied to the cookie.
The database stores a hash of the session ID while the browser keeps the
opaque value.

AuthMiddleware loads the cookie, resolves the session and user, and places the
user in request context. Invalid or expired sessions are deleted and redirected
to /login.

## CSRF behavior

Service.CSRFMiddleware protects unsafe methods for requests carrying a session
cookie. Safe methods are GET, HEAD, OPTIONS, and TRACE.

The request must provide the session's CSRF token as either:

- a csrf_token form field, or
- the X-CSRF-Token header.

Tokens are compared in constant time. Templates can render the token with
Auth.CSRFTokenInput or read it with Auth.CSRFToken.

Anonymous unsafe requests are not checked by this middleware. Login, signup,
and password-reset requests are consequently allowed to establish a session
without already having a CSRF token.

## Customer flow

On localhost:

1. GET|POST /signup validates the username, email, and password.
2. Passwords must be at least ten non-whitespace characters.
3. Passwords are hashed with bcrypt after appending a random per-user salt.
4. The account is created with role user.
5. GET|POST /login accepts only role user.
6. POST /logout deletes the current session and expires the cookie.

## Admin and staff flow

On admin.localhost:

- GET|POST /setup is available while no admin exists.
- Loopback requests may set up directly.
- Other requests require ADMIN_SETUP_TOKEN.
- GET|POST /login accepts admin and staff.
- GET|POST /staff/signup is admin-only.

The partner host reuses the admin login handler and accepts admin, staff, and
partner users. Partner users are scoped to their own products and orders;
admin and staff users retain global operational access.

## Password reset

Password-reset request and confirmation routes exist on all three hosts.
Reset tokens are random, stored hashed, one-time, and expire after 45 minutes.
Successful reset invalidates the user's existing sessions.

When a reset email sender is configured, the link is sent through SMTP. In
development, the flow can expose the reset link directly in the response.

## Abuse controls

The auth service contains an in-memory failure backoff keyed by IP and
identifier. It is disabled by default (AUTH_ABUSE_MAX_FAILURES=0).

The separate request limiter is configured with POST_RATE_LIMIT_MAX and
POST_RATE_LIMIT_WINDOW_SECONDS; see [known issues](08-known-issues.md) for its current
method-filtering defect.

## Security source files

- [session and auth service](../internal/auth/auth.go)
- [CSRF implementation](../internal/auth/csrf.go)
- [session persistence](../internal/db/sessions.go)
- [auth route registration](../internal/routes/register.go)

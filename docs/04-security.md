# Security

Most of this lives in `internal/auth`. HTTP hardening is in
`internal/app/middleware.go`, and the host allowlist is in
`cmd/sokomoko/server.go`.

## Accounts and passwords

- Usernames: 3 to 32 characters, starting with a letter or digit, then
  letters, digits, `.`, `_`, `-`. Unique case-insensitively.
- Emails are stored lower-cased and are unique.
- Passwords: 10 to 72 bytes, hashed with bcrypt at the default cost. Inputs
  longer than 72 bytes are rejected instead of being silently truncated.
- When a login names an unknown user, the password is still compared against
  a dummy hash, so response time does not reveal which usernames exist.
- Deactivating a user sets `users.deleted_at` and deletes their sessions.
  Every user lookup filters out deactivated users.

## Sessions

- Login creates a 32-byte random token, sent as the `session_token` cookie:
  `HttpOnly`, `SameSite=Lax`, `Path=/`, 24-hour lifetime.
- The cookie is `Secure` when the request is TLS, when a trusted proxy reports
  `X-Forwarded-Proto: https`, or whenever `ENV=production`.
- The database stores only the SHA-256 digest of the token (`sessions.id`), so
  a database leak does not reveal usable cookies. Password-reset tokens are
  stored the same way.
- Any session the browser already had is revoked at login, which prevents
  session fixation.
- Cookies are host-only by default. Setting `SESSION_COOKIE_DOMAIN` shares one
  session across the storefront, admin, and partner hosts; config validation
  requires every allowed host to sit under that domain.
- Logout is `POST` only. It deletes the session row and clears the cookie.

## Roles and authorization

Roles are `user`, `staff`, `admin`, `partner` (see
[Overview](01-overview.md#roles)). Authorization happens in two layers:

1. **Route wrappers** in `routes/register.go` and `api/handler.go`:
   `AuthMiddleware`, `RequireRole`, `RequireAnyRole`, `APIAuthMiddleware`.
2. **Service checks** for actions that need more than a role:
   - `admin.UpdateOrder` and `admin.DeactivateUser` require `admin`. Admins
     cannot deactivate themselves or other admins.
   - `partner.UpdateOrder` requires admin, staff, or partner. A partner may
     only update orders where every line belongs to their partner.
   - Partner product listing and creation are restricted to the caller's
     partner.

A `partner` user is linked to a vendor through the `partner_users` table.
`PartnerIDForUser` returns a partner only when the link exists, the partner is
`active`, and the user still has the `partner` role and is not deactivated.
With no valid link, a partner user sees empty lists.

## CSRF

`CSRFMiddleware` runs on every request:

- Safe methods (GET, HEAD, OPTIONS, TRACE) pass.
- Requests with no session cookie pass. Without a session there is nothing to
  forge, and this lets login and signup forms work before a session exists.
- With a session cookie, the request must carry the session's CSRF token in the
  `csrf_token` form field or the `X-CSRF-Token` header. The comparison is
  constant-time. A missing token gets `403 CSRF token missing`, a wrong one
  `403 CSRF token invalid` (JSON error codes on `/api/v1`).
- A cookie for an expired or revoked session is cleared and the request
  continues as anonymous, so a stale cookie cannot lock a browser out of
  `/login`. On the API it returns `403 invalid_session` instead.

Each session gets its own 32-byte CSRF token. Templates receive it through the
page data, and the base layout exposes it in `<meta name="csrf-token">` for
scripts.

## First-run admin setup

`/setup` on the admin host creates the first admin. It is available only while
no admin exists, and only when one of these holds:

- `ADMIN_SETUP_TOKEN` is set and the request supplies it in the
  `X-Admin-Setup-Token` header or the `setup_token` form field (constant-time
  compare; production requires at least 32 bytes); or
- no token is configured and the request comes from a loopback address.

The form can also create up to 20 staff accounts (`staff01`, `staff02`, ...)
with random temporary passwords that are shown once. The admin and staff are
inserted in one transaction that fails with `ErrAdminExists` if another request
won the race. Remove the setup token after setup.

For development there is an alternative: `seed` creates an admin from
`ADMIN_USERNAME`, `ADMIN_EMAIL`, and `ADMIN_PASSWORD` when no admin exists.

## Password reset

1. `/password-reset/request` accepts a username or email. The response is the
   same whether or not the account exists.
2. If the account exists and its role is allowed on that host, a 32-byte token
   valid for 45 minutes is stored (as a hash) and any older token for the user
   is deleted.
3. The link is built from `PASSWORD_RESET_BASE_URL` when set, otherwise from
   the request host. Production requires the base URL, so reset links never
   trust the `Host` header.
4. The email is sent in a background goroutine with a 5-second timeout through
   `notify.SMTPSender`. Production requires STARTTLS. Send failures are
   logged, not shown to the user.
5. Outside production the link is also shown on the page, so local
   development works without SMTP.
6. `/password-reset/confirm` checks the token and role again. Using it sets the
   new hash, marks the token used, and revokes every session of that user, all
   in one transaction.

## Throttling

Two independent, in-memory mechanisms:

| Mechanism | Scope | Config | Behaviour |
| --- | --- | --- | --- |
| Auth abuse limiter | Login (per area), reset request, reset confirm | `AUTH_ABUSE_MAX_FAILURES`, `..._BACKOFF_BASE_SECONDS`, `..._BACKOFF_MAX_SECONDS` | Keyed by scope, client IP, and identifier. After the maximum number of failures, locks for the base duration, doubling per further failure up to the maximum. Success clears the key. Locked requests get `429` with `Retry-After`. Off when max failures is 0. |
| POST rate limit | Every POST | `POST_RATE_LIMIT_MAX`, `POST_RATE_LIMIT_WINDOW_SECONDS` | Fixed window per client IP. `429` with `Retry-After`. Off when max is 0. |

Client IP is `RemoteAddr`, or the first `X-Forwarded-For` entry when
`TRUST_PROXY_HEADERS=true`. Enable that only behind a proxy that overwrites the
header, or clients can spoof their IP to dodge limits.

Both limiters live in process memory, so limits are per instance and reset on
restart.

## HTTP hardening

- **Host allowlist.** Unknown `Host` headers get `400`.
- **Headers on every response:** `Content-Security-Policy` (`default-src 'self'`,
  scripts from self only, inline styles allowed, `frame-ancestors 'none'`),
  `X-Frame-Options: DENY`, `X-Content-Type-Options: nosniff`,
  `Referrer-Policy: strict-origin-when-cross-origin`, `Cache-Control: no-store`.
  HSTS (one year, subdomains) is added for HTTPS requests.
- **Body limit** of 1 MiB on unsafe methods.
- **Open-redirect guard.** The login `next` parameter must be a same-site
  relative path (no `//`, backslashes, or line breaks; at most 512 bytes).
- **Flash messages** travel in a 60-second `HttpOnly` cookie (base64 JSON, at
  most 500 characters, kind limited to success, danger, or info) rather than in
  query strings, so a crafted link cannot make the site display arbitrary text.
  The cookie is not signed; it is only ever set by the server.
- **Static files** reject `..` segments and resolve only inside
  `internal/ui/static`.
- **Product images** with remote URLs are never hot-linked; they fall back to a
  bundled image, which keeps `img-src 'self'` intact.
- **Request IDs** from clients are accepted only when short and alphanumeric, so
  they cannot forge log lines.
- **Logs** contain no query strings, cookies, passwords, tokens, or addresses.

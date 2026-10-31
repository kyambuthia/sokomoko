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

- Login creates a 32-byte random token, sent in an `HttpOnly`,
  `SameSite=Lax`, `Path=/` cookie with a 24-hour lifetime.
  Secure host-only requests use `__Host-session_token`, which browsers
  reject if a sibling subdomain tries to set it for the parent domain. Secure
  shared-domain requests use `__Secure-session_token` to reject cookies
  injected over plain HTTP; it does not protect against HTTPS sibling hosts.
  Secure deployments replace existing `session_token` cookies with these
  names, so users sign in again after upgrading to this version.
- The cookie is `Secure` when the request is TLS, when a trusted proxy reports
  `X-Forwarded-Proto: https`, or whenever `ENV=production`.
- The database stores only the SHA-256 digest of the token (`sessions.id`), so
  a database leak does not reveal usable cookies. Password-reset tokens are
  stored the same way.
- Any session the browser already had is revoked at login, which prevents
  session fixation.
- Cookies are host-only by default. Setting `SESSION_COOKIE_DOMAIN` shares one
  session across the storefront, admin, and partner hosts; config validation
  requires every allowed host to sit under that domain. Shared-domain mode
  trusts every sibling subdomain that can set cookies for that domain, including
  hosts outside `ALLOWED_HOSTS`. Use it only when all such subdomains are
  controlled by the same security boundary.
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
     only update orders where every line belongs to their partner. Ownership is
     checked in SQL (`Store.PartnerOwnsOrder`, `Store.ListOrdersForPartner`).
   - Partner product listing and creation are restricted to the caller's
     partner.
   - `partner.SaveStoreSettings` requires admin or staff, because the store
     settings are marketplace-wide. Partner users see them read-only.

A `partner` user is linked to a vendor through the `partner_users` table.
`PartnerIDForUser` returns a partner only when the link exists, the partner is
`active`, and the user still has the `partner` role and is not deactivated.
With no valid link, a partner user sees empty lists.

## CSRF

`CSRFMiddleware` runs on every request:

- Safe methods (GET, HEAD, OPTIONS, TRACE) pass.
- Anonymous requests, including those with an expired or revoked session
  cookie, do not need a session token. Browser Origin or Referer metadata
  must match the request origin, and Sec-Fetch-Site must not indicate a
  cross-origin request. This protects login, signup, and password-reset forms.
  Clients that omit browser origin metadata remain supported.
- With a valid session cookie, the request must carry the session's CSRF token in the
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
- no token is configured and the request comes straight from a loopback
  address. A request carrying any forwarding header (`X-Forwarded-For`,
  `X-Real-Ip`, `Forwarded`, `Via`, ...) never counts as loopback, because a
  reverse proxy on the same host relays internet traffic from loopback too.
  Behind a proxy, set `ADMIN_SETUP_TOKEN`.

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

Authentication lockouts, POST limits, and password-reset cooldowns use PostgreSQL:

| Mechanism | Scope | Config | Behaviour |
| --- | --- | --- | --- |
| Auth abuse limiter | Login (per area), reset request, reset confirm | `AUTH_ABUSE_MAX_FAILURES`, `..._BACKOFF_BASE_SECONDS`, `..._BACKOFF_MAX_SECONDS` | PostgreSQL-shared exponential backoff keyed by an HMAC of scope, client IP, and identifier, using `DATABASE_URL` as shared key material. Failed login/reset-confirm attempts count; reset requests count every non-empty identifier equally. Successful login/confirmation clears the key. DB errors fall back to the bounded process-local limiter. Locked requests get `429` with `Retry-After`. Off when max failures is 0. |
| POST rate limit | Every POST | `POST_RATE_LIMIT_MAX`, `POST_RATE_LIMIT_WINDOW_SECONDS` | PostgreSQL-shared fixed window per client IP using an HMAC digest keyed from the shared `DATABASE_URL`. `429` with `Retry-After`; process-local fallback during database errors. Off when max is 0. |
| Reset email cooldown | Per user across instances | None | PostgreSQL allows one reset email every five minutes. Requests during the cooldown preserve the current link and send no additional email. |

Client IP is `RemoteAddr`, or the right-most `X-Forwarded-For` entry when
`TRUST_PROXY_HEADERS=true`. The right-most entry is the one the proxy wrote, so
values a client puts in the header cannot change its rate-limit key. This
assumes exactly one trusted proxy in front of the app.

Authentication lockouts and POST counters are shared across instances. If a database
call fails, both limiters use their bounded process-local fallback. Auth lockout rows
use a logged table and follow the database replication durability settings; POST
counters use an unlogged table and can reset after a database crash or failover.

Messaging also has persistent PostgreSQL budgets for contact lookups,
conversation creation, sends and reports. These are shared across processes.
Every messaging operation checks user, shop or delivery membership, including
polling. A courier reassignment revokes the former courier's chat access.
Admins can review only messages submitted through the report flow, with no
implicit access to private threads. New message content uses browser-side MLS
end-to-end encryption; the server stores ciphertext and routing metadata.
Earlier plaintext history remains separately labelled. Later devices require
approval by an existing device and receive future sessions only. Shared demo
accounts are public identities and must never carry private information.
See [Encrypted messaging](encrypted-messaging.md) for the trust model,
verification, recovery limitations and recipient-disclosed reports.

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

## Workspace service boundaries

Partner dashboard, catalogue, order queries and product creation require a
positive actor ID with an explicit admin, staff or partner role inside the
service, in addition to HTTP route guards. Missing actors, buyers, couriers,
unknown roles and nonpositive IDs cannot fall through into platform-wide
scope. Product edits keep the not-found response for unauthorized actors.
Marketplace-wide settings changes also require a positive admin/staff ID.
Actor records are trusted application inputs built from authenticated users;
these checks do not replace authentication or database membership checks.

Courier dispatch operations and rider operations have separate role checks.
Courier history retains completed jobs but the shared delivery query returns an
empty address after completion, for both HTML and JSON responses. Active jobs
and failed attempts retain the address for delivery work. This is a read
projection; the original address remains in the order record. Administrative
completion through either the order status or delivery status also hides the
address, removes the job from workload counts and closes courier messaging and
handoff actions. Existing conversation history remains available to authorized
participants. The courier event history is not rewritten to imply a rider
confirmed an administrative completion. Courier deactivation is blocked while
a nonterminal order has an assigned, picked-up or out-for-delivery handoff.
Cancelled and completed orders do not block deactivation; successful
deactivation still revokes every session.
A status retry is a no-op only for the current owning courier, for the same
current action state (pickup, out for delivery, delivery or failure), and with
the same stored failure category. Ownership and cancellation are checked
before retry handling. Retries neither append an audit event nor change
stored timestamps. A request for an earlier state after the job has advanced
still fails; this is not a general request-idempotency-key system.

References: [OWASP authorization guidance](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html)
and [PostgreSQL 17 row locks](https://www.postgresql.org/docs/17/explicit-locking.html).

Courier job views (`all`, `active`, `completed`) use the same ownership scope
and completion checks. Filtering occurs before the 200-job limit. Completed
history is ordered by newest assignment first; active work keeps oldest
assignments first. Unknown views return 400. The default remains all jobs.
Status forms retain only an allowlisted view value on their local redirect.

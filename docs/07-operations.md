# Operations

## Commands

| Command | Behavior |
| --- | --- |
| go run ./cmd/sokomoko serve | Apply schema and run the HTTP server |
| go run ./cmd/sokomoko serve --seed | Apply schema, seed, then run the server |
| go run ./cmd/sokomoko migrate | Apply schema and exit |
| go run ./cmd/sokomoko seed | Apply schema, seed, and exit |

The server listens on PORT, default 6969. SQLite uses DB_PATH, default
./db/t.db.

## Trusted hosts

The default allowed hosts are:

- localhost
- 127.0.0.1
- admin.localhost
- partner.localhost

Set ALLOWED_HOSTS to replace the default list. Requests with a host outside
the list receive 400 before route handling.

## HTTP hardening

The live middleware chain includes:

- panic recovery and an error page
- request IDs
- security headers and a content-security policy
- a one-megabyte limit for POST, PUT, and PATCH bodies
- token-based CSRF validation for cookie-authenticated unsafe requests
- optional in-memory IP rate limiting
- request logging

The HTTP server has a 15-second read and write timeout, a 10-second header
timeout, and a 60-second idle timeout. Shutdown waits up to 30 seconds.

## Cookies and HTTPS

Sessions use Secure cookies when the request is TLS, when
X-Forwarded-Proto is https, or when ENV=production. If a reverse proxy
terminates TLS, it must pass the correct X-Forwarded-Proto value.

SESSION_COOKIE_DOMAIN can be set for a shared parent domain. Use it only when
the deployment deliberately shares the session across storefront, admin, and
partner subdomains.

## Password-reset email

SMTP is optional. SMTP_HOST and SMTP_FROM enable an SMTP sender; SMTP_PORT
defaults to 587. If the sender cannot be constructed, startup logs that email
delivery is disabled and the server continues running.

PASSWORD_RESET_BASE_URL controls the base used to build reset links. Verify
SMTP and reset-link configuration before treating password reset as a
production capability.

## Background work

The server runs cleanup immediately at startup and every five minutes after
that. Cleanup removes expired sessions and password-reset tokens. There is no
separate worker process.

## Health checks

- /healthz checks liveness.
- /readyz pings the database.

Both endpoints are registered on each host mux.

## Seed and deployment cautions

- serve --seed is intended for disposable development data.
- Admin seed credentials come from ADMIN_PASSWORD and optional admin identity
  variables.
- Partner seed passwords are hardcoded in the current source.
- There is no built-in backup, restore, metrics, or structured-log pipeline.
- Incremental schema migrations are not implemented.

## Verification

Before deployment:

    go test ./...
    go vet ./internal/... ./cmd/...
    go build ./cmd/sokomoko

Also verify trusted hosts, HTTPS forwarding, session cookie behavior, database
backups, SMTP delivery, and the real payment integration status.

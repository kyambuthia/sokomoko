# Operations

## Commands

| Command | Behavior |
| --- | --- |
| go run ./cmd/sokomoko serve | Verify schema in production; migrate in development; run server |
| go run ./cmd/sokomoko serve --seed | Apply schema, seed, then run the server |
| go run ./cmd/sokomoko migrate | Apply schema and exit |
| go run ./cmd/sokomoko seed | Apply schema, seed, and exit |

The server listens on PORT, default 6969. PostgreSQL uses DATABASE_URL;
production requires an explicit connection string. See [.env.example](../.env.example).

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
trusted X-Forwarded-Proto is https, or when ENV=production. Set
TRUST_PROXY_HEADERS only behind a proxy that overwrites forwarding headers. If a reverse proxy
terminates TLS, it must pass the correct X-Forwarded-Proto value.

SESSION_COOKIE_DOMAIN can be set for a shared parent domain. Use it only when
the deployment deliberately shares the session across storefront, admin, and
partner subdomains.

## Password-reset email

SMTP is optional in development and required in production. Invalid configured
SMTP fails startup. Production requires STARTTLS, an HTTPS reset origin, and
trusted hosts. Delivery uses a bounded SMTP operation.

## Background work

Maintenance releases expired reservations every minute and prunes expired
sessions/reset tokens every five minutes. Shutdown drains HTTP and cancels
maintenance before closing the database pool.

## Health checks

- /healthz checks liveness.
- /readyz pings the database.

Both endpoints are registered on each host mux.

## Releases and recovery

Production rejects demo seeding. Use the [deployment runbook](DEPLOYMENT.md)
for TLS, separate migration credentials, backup, restore, and rollback.
Structured logs and production image smoke tests are included; live deployment
validation and external service checks remain environment-specific.

## Verification

With a disposable PostgreSQL test database configured:

    SOKOMOKO_REQUIRE_DB_TESTS=1 go test -race ./...
    go vet ./...
    go build ./cmd/sokomoko

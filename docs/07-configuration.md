# Configuration

`internal/config` reads environment variables only; it does not load `.env`
files. `.env.example` lists every variable for local use and
`deploy/production.env.example` for production.

`Config.Validate` runs at `serve` startup and reports every problem at once.
`migrate` and `seed` run only `ValidateDatabase`, so release jobs do not need
SMTP or host settings.

## Core

| Variable | Default | Notes |
| --- | --- | --- |
| `ENV` | `development` | `development`, `test`, or `production`. Production turns on strict validation, `Secure` cookies, JSON logs, schema check instead of auto-migrate, and refuses seeding. |
| `BIND_HOST` | empty (all interfaces) | Optional unbracketed IPv4 or IPv6 listener address. `make run` defaults to `127.0.0.1`; override with `make run BIND_HOST=0.0.0.0` for LAN access. Containers must listen on their network interface; local Compose explicitly uses `0.0.0.0` and publishes only to host loopback. This controls the listener, independently of `ALLOWED_HOSTS`. |
| `PORT` | `6969` | 1 to 65535. The container image sets `8080`. |
| `LOG_FORMAT` | `text` (`json` in production) | `json` or `text` |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |

## Metrics

| Variable | Default | Notes |
| --- | --- | --- |
| `METRICS_TOKEN` | empty (disabled) | Optional Bearer token for `GET/HEAD /metrics`. At least 32 bytes with no whitespace; generate one with `openssl rand -hex 32`. The endpoint is served only for allowed hosts and has no path, host, or user labels. |

## Database

| Variable | Default | Notes |
| --- | --- | --- |
| `DATABASE_URL` | `postgres://sokomoko@localhost:5432/sokomoko?sslmode=disable` | Required explicitly in production |
| `DB_MAX_OPEN_CONNS` | `20` | At least 2: a maintenance pass holds one connection for its lock while the job uses another |
| `DB_MAX_IDLE_CONNS` | `10` | Must not exceed max open |
| `DB_CONN_MAX_LIFETIME_SECONDS` | `1800` | |
| `DB_QUERY_TIMEOUT_SECONDS` | `10` | Upper bound for every store call |

## Hosts, proxy, and cookies

| Variable | Default | Notes |
| --- | --- | --- |
| `ALLOWED_HOSTS` | `localhost,127.0.0.1,admin.localhost,partner.localhost` when empty | Comma-separated exact hostnames, no scheme, port, path, or wildcard. Required in production. Admin and partner hosts must start with `admin.` and `partner.`. The first entry is used as the `Host` for `sokomoko healthcheck`. |
| `TRUST_PROXY_HEADERS` | `false` | Trust `X-Forwarded-For` (right-most entry) and `X-Forwarded-Proto`. Only behind exactly one reverse proxy. |
| `SESSION_COOKIE_DOMAIN` | empty (host-only cookies) | A DNS domain that contains every allowed host, for example `.example.com`, to share sessions across hosts. Sharing trusts every sibling subdomain that can set cookies for that domain. |

## Admin bootstrap

| Variable | Default | Notes |
| --- | --- | --- |
| `ADMIN_SETUP_TOKEN` | empty | Unlocks `/setup` on the admin host. At least 32 bytes in production. When empty, `/setup` is allowed only from a direct loopback request (never through a proxy). |
| `ADMIN_USERNAME`, `ADMIN_EMAIL`, `ADMIN_PASSWORD` | `admin`, `admin@sokomoko.local`, empty | Read by `seed` only. No admin is seeded unless `ADMIN_PASSWORD` (10 to 72 bytes) is set. |

## Abuse protection

| Variable | Default | Notes |
| --- | --- | --- |
| `POST_RATE_LIMIT_MAX` | `0` (off) | POST requests per client IP (IPv4 address or IPv6 /64) per window, shared across replicas through PostgreSQL |
| `POST_RATE_LIMIT_WINDOW_SECONDS` | `60` | Must be positive when the limit is on |
| `RATE_LIMIT_HMAC_KEY` | `DATABASE_URL` | Optional shared secret for rate-limit and authentication-abuse digests. Set a random value such as `openssl rand -hex 32` and keep it identical across replicas. Rotating it resets active limiter keys. |
| `AUTH_ABUSE_MAX_FAILURES` | `0` (off) | Shared lockout threshold for login failures, reset-request attempts, and reset-confirm failures |
| `AUTH_ABUSE_BACKOFF_BASE_SECONDS` | `1` | First lockout |
| `AUTH_ABUSE_BACKOFF_MAX_SECONDS` | `300` | Lockout cap; must be at least the base |

## Password reset and email

| Variable | Default | Notes |
| --- | --- | --- |
| `PASSWORD_RESET_BASE_URL` | empty (use request host) | Absolute `http(s)` origin with no path, query, or credentials. Production requires HTTPS and a host from `ALLOWED_HOSTS`, unless password reset is disabled. |
| `PASSWORD_RESET_DISABLED` | `false` | Hides reset links and returns 404 from reset endpoints. Production can omit the reset URL and SMTP when enabled; signed-in password changes still work. |
| `SMTP_HOST`, `SMTP_FROM` | empty | Set together. Required in production when password reset is enabled. |
| `SMTP_PORT` | `587` | |
| `SMTP_USERNAME`, `SMTP_PASSWORD` | empty | Set together; authenticated SMTP requires STARTTLS in every environment. |

When neither `SMTP_HOST` nor `SMTP_FROM` is set, no email is sent. Outside
production the reset link is shown on the page instead.

## Demo mode

| Variable | Default | Notes |
| --- | --- | --- |
| `DEMO_MODE` | `false` | Shows a notice that listings are sample content and blocks order and payment flows. |

## Online payments

| Variable | Default | Notes |
| --- | --- | --- |
| `PUBLIC_BASE_URL` | `PASSWORD_RESET_BASE_URL` | Storefront origin for provider return and callback URLs. Production requires HTTPS and an allowed host. |
| `SECRETS_KEY` | empty | Base64 encoding of 32 random bytes (`openssl rand -base64 32`). Production requires it when password-reset email is enabled so queued reset links are encrypted; it also seals provider credentials. Development uses a fixed local key when empty. |
| `SECRETS_KEY_PREVIOUS` | empty | Previous key accepted for decryption during a planned rotation; new credentials are always sealed with `SECRETS_KEY`. |

Keep the key with encrypted database backups. To rotate it, back up the
database and old key, set a new `SECRETS_KEY` and the old value as
`SECRETS_KEY_PREVIOUS`, then restart. Re-enter each enabled provider's secret
fields in the admin payment settings so they are sealed under the new key.
Keep the previous key available for at least 45 minutes after rotation, or
until every pending password-reset email has drained; otherwise those queued
links cannot be decrypted. After that, restart with only the new key to confirm
every enabled provider opens. Production startup checks enabled online providers,
their required fields, store currency, and public callback origin before
listening.

## Tests

| Variable | Notes |
| --- | --- |
| `SOKOMOKO_TEST_DATABASE_URL` | Database the tests may create and drop schemas in. Unset means database tests are skipped. |
| `SOKOMOKO_REQUIRE_DB_TESTS` | `1` makes a missing test database a failure instead of a skip (set by `make test` and CI). |

## Production checklist enforced at startup

With `ENV=production`, `serve` refuses to start unless:

- `DATABASE_URL` is set explicitly;
- `ALLOWED_HOSTS` is non-empty;
- unless `PASSWORD_RESET_DISABLED=true`, `PASSWORD_RESET_BASE_URL` is set,
  uses HTTPS, and names an allowed host;
- unless password reset is disabled, `SMTP_HOST` and `SMTP_FROM` are set (and
  the server supports STARTTLS when a mail is actually sent);
- `ADMIN_SETUP_TOKEN`, if set, is at least 32 bytes;
- every embedded migration has been applied and the database has no unknown
  versions.

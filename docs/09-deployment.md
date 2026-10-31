# Deployment

The repository ships a reference production setup: a distroless container
image, a Compose file with a one-shot migration job, and Caddy as the TLS
reverse proxy.

```text
 Internet ──▶ Caddy :80/:443 (TLS, HSTS, compression, X-Forwarded-*)
                 │
                 ▼
             app :8080 (sokomoko serve, read-only container, no published port)
                 │
                 ▼
             PostgreSQL 17 (managed, outside this Compose file)

 Release: migrate (one-shot, DDL-capable role) runs before app starts
```

## Container image

`Dockerfile`:

- Build stage on `golang:1.26.10-bookworm`. `CGO_ENABLED=0`, `-trimpath`,
  `-ldflags "-s -w -X main.version=${VERSION}"`.
- Runtime stage `gcr.io/distroless/static-debian12:nonroot`: only the binary,
  running as `nonroot`.
- Defaults: `ENV=production`, `PORT=8080`, `LOG_FORMAT=json`.
- `HEALTHCHECK` runs `sokomoko healthcheck` every 30 s.
- `ENTRYPOINT ["/usr/local/bin/sokomoko"]`, `CMD ["serve"]`.

`.dockerignore` keeps `.git`, `docs`, local binaries, databases, logs, and
`.env*` files out of the build context.

`scripts/smoke-image.sh` (run in CI) tests an image in isolated, disposable
containers. It migrates twice to prove idempotency, creates a restricted
runtime role, starts the app read-only with all capabilities dropped, checks
readiness on all three hosts, confirms the runtime role has no DDL rights and
that production did not seed data, stops the app with `SIGTERM`, and verifies a
dump and restore into a fresh database.

## Production Compose

`deploy/compose.production.yml` defines three services sharing one network:

| Service | Purpose | Notes |
| --- | --- | --- |
| `migrate` | `sokomoko migrate` | `release` profile, run on demand, uses `MIGRATION_DATABASE_URL` |
| `app` | `sokomoko serve` | `read_only`, `cap_drop: ALL`, `no-new-privileges`, 512 MiB, 1 CPU, 128 PIDs, 45 s stop grace period |
| `proxy` | Caddy 2.11 | Publishes 80 and 443 (TCP and UDP); waits for the app to be healthy |

The image is pinned by `SOKOMOKO_IMAGE`, which should be a verified digest.
`deploy/Caddyfile` serves the configured hostnames, obtains certificates,
adds HSTS, and overwrites `X-Forwarded-For` and
`X-Forwarded-Proto`. That is why the app runs with `TRUST_PROXY_HEADERS=true`;
it is safe only while Caddy is the sole way in. On the EC2 demo, the instance
security group allows TCP 80 and 443 only from Cloudflare's published IPv4
ranges. This keeps direct requests from reaching Caddy with forged proxy
headers or bypassing Cloudflare edge controls. Port 80 remains available through
Cloudflare for HTTP-01 certificate validation.

Cloudflare publishes the current ranges at
<https://api.cloudflare.com/client/v4/ips>. If the ranges change, add the new
CIDRs to both ports before removing retired CIDRs, then verify all proxied
hostnames still return `/readyz` successfully. Keep the DNS records orange-cloud
proxied; the demo EC2 instance has no IPv6 address.

Copy `deploy/production.env.example` to `.env.production`, fill in real values,
and restrict its permissions. Set `COURIER_HOST` to the courier workspace
hostname and create a matching proxied DNS record. `DOCS_HOST` is the public
feature guide host. Admin, partner, courier, and docs hostnames must use the
`admin.`, `partner.`, `courier.`, and `docs.` prefixes because the router
dispatches on them. The docs host serves the embedded public guide and does
not expose the JSON API.

### Single-instance demo

`deploy/compose.ec2-demo.yml` adds a local PostgreSQL 17 container for a
single-instance demo. The database has no published port and stores its data
in a Docker volume on the host's encrypted EBS disk. This is a low-cost demo
layout, not a replacement for managed PostgreSQL backups and recovery.

For a demo without email, set `PASSWORD_RESET_DISABLED=true` and leave the
SMTP variables empty. Production startup then allows SMTP to be omitted,
password-reset links disappear from sign-in pages, and reset URLs return 404.
Users can still change a password after signing in.

Set `DEMO_MODE=true` to show a site-wide notice that listings are sample
content and that no sales, orders, or payments are accepted. The checkout
page explains that it is disabled, web order submission returns 403, payment
routes are unavailable, and API checkout endpoints return a demo-mode error.

The `seed` Compose profile is a one-time initialization step for the demo. It
runs the catalog seed and creates a customer account when
`DEMO_CUSTOMER_PASSWORD` is set and a partner account when
`DEMO_PARTNER_PASSWORD` is set. Run it with `ENV=development` in the isolated
demo database; the public production server still refuses seed-on-start.
Repeated seeding leaves existing accounts unchanged.

The seed command can also create an admin from `ADMIN_USERNAME`,
`ADMIN_EMAIL`, and `ADMIN_PASSWORD`. Keep that credential private. When no
admin is created, `/setup` remains available only to loopback requests, so it
cannot be claimed through the public reverse proxy.

Courier accounts are created by an admin from the admin deliveries page. Each
account uses a username and initial password, with no email address or reset
email. Share initial passwords out of band and have couriers change them after
sign-in. The demo seeds no courier users or deliveries, so the courier
workspace has no public demo credentials. Dispatch is manual; the application
does not provide GPS tracking or route optimization. Messaging between buyers
and couriers is available for assigned deliveries; see [Messaging](messaging.md).

## Database roles

Use two roles:

- a **migration role** with DDL rights, used only by the `migrate` job;
- a **runtime role** with DML on application tables and read-only access to
  `schema_migrations`.

After the first migration, grant the runtime role (adjust names):

```sql
GRANT USAGE ON SCHEMA public TO sokomoko_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO sokomoko_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO sokomoko_app;
REVOKE INSERT, UPDATE, DELETE ON public.schema_migrations FROM sokomoko_app;
ALTER DEFAULT PRIVILEGES FOR ROLE sokomoko_migrate IN SCHEMA public
  GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO sokomoko_app;
ALTER DEFAULT PRIVILEGES FOR ROLE sokomoko_migrate IN SCHEMA public
  GRANT USAGE, SELECT ON SEQUENCES TO sokomoko_app;
```

## Release procedure

```bash
compose="docker compose --env-file .env.production -f deploy/compose.production.yml"

$compose config --quiet                        # validate without printing secrets
# For migration 0011, stop all old app replicas before migrating (see below).
$compose --profile release run --rm migrate    # apply migrations
$compose up -d app proxy                       # start or roll the app
$compose ps
$compose exec -T app /usr/local/bin/sokomoko healthcheck
```

The production app does not migrate on startup. It refuses to start if the
database is behind the binary or has versions the binary does not know.
Keep migrations additive so the previous binary keeps working while the new
one rolls out. Review semantic compatibility as well as SQL compatibility:
adding a new context to an existing query can change how an older process reads
new rows even when its statements still execute.

**Migration 0011 requires a coordinated application restart.** It adds service
conversation identity to the shared seller-conversation index and lookup. An old
process does not filter this new context and could resolve the wrong thread when
opening a general shop conversation. Stop **all** old app replicas before running
this migration, then start only the new image. In the single-host Compose setup,
insert `$compose stop app` before the migration command above. This creates a
brief maintenance interval. Do not perform a mixed-version rolling deployment
for this release, and do not start an old binary against schema 11.

First deployment only: open `/setup` on the admin host with the setup token,
create the administrator, then remove `ADMIN_SETUP_TOKEN` and recreate the app
container. `seed` and `serve --seed` are refused in production.

## Health and shutdown

- `/healthz`: process is up. `/readyz`: database reachable and not draining.
- `sokomoko healthcheck` requests `http://127.0.0.1:$PORT/readyz` with the
  first allowed host as `Host`, ignores proxy environment variables, does not
  follow redirects, and times out after 3 s.
- On `SIGTERM`: readiness turns 503, background maintenance stops, in-flight
  requests get up to 30 s, then the store closes. Compose allows 45 s.

## Observability

- Logs go to stderr as JSON in production: one line per request with method,
  path, host, status, duration, and request ID, plus startup, maintenance, and
  error lines.
- Every response has `X-Request-Id`; error pages show it.
- There is no metrics endpoint or tracing. Watch 5xx rate, latency, readiness
  failures, database connections, disk, backup age, and SMTP send errors from
  the platform side.

## Scaling

POST limits are shared through PostgreSQL across app instances, with a
process-local fallback during database errors. Authentication abuse lockouts
remain per instance, so extra replicas multiply the attempts allowed before an
account or client is locked. Maintenance jobs use a PostgreSQL advisory lock,
so they can run on every replica while only one performs each pass.

## Backups and recovery

The repository does not run backups. Prefer a managed PostgreSQL with
point-in-time recovery. For logical backups:

```bash
umask 077
PGSERVICE=sokomoko_backup pg_dump --format=custom --no-owner --no-acl --file=sokomoko.dump
pg_restore --list sokomoko.dump
```

Restore into a **new, isolated database**, never over the live one:

```bash
PGSERVICE=sokomoko_restore pg_restore --exit-on-error --single-transaction \
  --no-owner --no-acl --dbname=sokomoko_restore sokomoko.dump
```

Then re-apply the runtime grants, start the matching binary against it to
verify schema history, and check orders, payments, and inventory before
switching over. Keep credentials in protected PostgreSQL service and password
files, not on the command line.

**Rollback.** For an app-only change, redeploy the previous image digest. If a
migration was applied, the old binary will refuse to start against the newer
schema; prefer a forward fix over editing migration history.

**Secret rotation.** Create the new credential, update the secret store,
recreate the app, confirm readiness, then revoke the old credential.

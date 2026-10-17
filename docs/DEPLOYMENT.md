# Production deployment runbook

This reference deployment runs one Go application behind Caddy, using an
external PostgreSQL database and SMTP provider. It supports cash on delivery.
The partner host is an internal platform workspace; give access only to trusted
platform staff until vendor isolation and per-partner fulfillment are built.

## Prepare the environment

1. Provision PostgreSQL 17 with automated backups and point-in-time recovery.
   Use `sslmode=verify-full` for remote connections. Keep the database private
   and permit connections only from the application and release runner.
2. Create separate database roles for migrations (schema owner) and application
   DML. Provision their passwords through the platform's secret store.
3. Configure an SMTP sender that supports STARTTLS on port 587. Test delivery
   with the provider, including sender verification and SPF/DKIM configuration.
4. Point `shop`, `admin`, and `partner` hostnames to the proxy. Admin and partner
   hostnames must begin with `admin.` and `partner.` because routing uses those
   prefixes. Open ports 80 and 443 for Caddy's HTTPS setup.
5. Build an image from a reviewed commit, put it in your registry, and record
   its digest. Use the same digest for migration and application containers.
   CI builds and tests images; it does not publish or deploy them automatically.

```bash
docker build --build-arg VERSION="$(git rev-parse HEAD)" -t YOUR_REGISTRY/sokomoko:YOUR_RELEASE .
```

Populate configuration without committing credentials:

```bash
cp deploy/production.env.example .env.production
chmod 600 .env.production
```

Set every example value, including the tested `SOKOMOKO_IMAGE` digest, runtime
and migration database URLs, DNS names, SMTP credentials, and a random first-run
setup token with at least 32 bytes of entropy. Compose's env file parser does
not execute commands. Generate the token separately and store it securely.
Use a secret manager to supply environment variables on managed platforms.

Keep `SESSION_COOKIE_DOMAIN` empty for separate host sessions unless shared
sessions across all three hosts are a deliberate requirement. The runtime app
receives only its database URL; migration credentials go only to the release
container. Database connection strings and passwords never belong in logs.

## Release

Run from the repository root. Config validation below does not print resolved
configuration, which would include secrets.

```bash
docker compose --env-file .env.production -f deploy/compose.production.yml config --quiet
docker compose --env-file .env.production -f deploy/compose.production.yml --profile release run --rm migrate
```

After the first migration, the database administrator must grant the runtime
role access. Adapt schema and role names to your database:

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

Do not grant CREATE on the application schema to the runtime role. Migrations
should be additive and compatible with the old running binary during release.
Destructive changes require a maintenance window and a recovery plan.

```bash
docker compose --env-file .env.production -f deploy/compose.production.yml up -d app proxy
docker compose --env-file .env.production -f deploy/compose.production.yml ps
docker compose --env-file .env.production -f deploy/compose.production.yml exec -T app /usr/local/bin/sokomoko healthcheck
```

Visit `/setup` on the admin host using the configured setup token, create the
administrator, then remove `ADMIN_SETUP_TOKEN` and recreate the app container.
Use only trusted platform admins and staff in the operations workspace.
`seed` and `serve --seed` refuse to run in production.

The app listener has no published host port. Caddy overwrites forwarded client
IP and protocol headers, terminates TLS, and persists certificate state in its
data volume. `TRUST_PROXY_HEADERS=true` is safe only while the app is reachable
exclusively through this proxy. An additional load balancer requires a deliberate
trusted-proxy configuration; this reference assumes Caddy is the public edge.

## Health, shutdown, and monitoring

- `/healthz` checks the process. `/readyz` checks database connectivity and
  returns unavailable while draining. Both support GET and HEAD.
- The container runs `sokomoko healthcheck`, which checks `/readyz` locally using
  an allowed Host header. It rejects redirects and has a three-second deadline.
- On SIGTERM the app stops accepting requests, cancels maintenance SQL, and
  drains active requests for up to 30 seconds. Compose allows 45 seconds before
  force termination. The store closes after maintenance has stopped.
- Logs are JSON, include request IDs and response status, and omit query strings,
  cookies, passwords, and reset tokens. Request logs include CSRF and throttle
  failures. Shipping addresses are not written to request logs.
- Monitor 5xx rates, response latency, failed readiness probes, DB connections,
  disk growth, backup age, and SMTP errors. Container health alone does not
  provide alerting or automatically restart an unhealthy Compose service.

Run one application instance initially. Auth and POST limits are per process.
Maintenance is safe to run concurrently but is duplicated across replicas.
Horizontal scaling requires shared abuse controls and cluster job coordination.

## Backups, restore, and rollback

Set explicit recovery objectives before launch (for example a 15-minute RPO
with managed WAL archiving and a one-hour RTO). These are targets to demonstrate
with restore drills, not guarantees provided by this repository.

Prefer managed point-in-time recovery and encrypted off-host backups. For a
logical backup, use a PostgreSQL 17 client or a newer compatible client and a
restricted backup role with access to every application table. Supply credentials
through a protected PostgreSQL service/password file. Avoid passwords in command
arguments or shell history. `PGSERVICE` below refers to local protected service
definitions, not values committed to this repository.

```bash
umask 077
PGSERVICE=sokomoko_backup pg_dump --format=custom --no-owner --no-acl --file=sokomoko.dump
pg_restore --list sokomoko.dump
```

Schedule backups, encrypt and copy them off the host, alert on failure, and
define retention (for example 7 daily and 4 weekly copies). A directory listing
does not verify recoverability. Restore to a **new, isolated database** using
a separately configured restore service and the DDL-capable role:

```bash
PGSERVICE=sokomoko_restore pg_restore --exit-on-error --single-transaction --no-owner --no-acl --dbname=sokomoko_restore sokomoko.dump
```

The restore service should target the isolated restore server, and
`sokomoko_restore` must name the new database on that server. Restore runtime role
grants, verify schema history with the matching binary, check order/payment
totals and inventory constraints, then exercise login and checkout before
cutting over. Record elapsed recovery time and the newest recovered order.

To roll back an app-only change, restore the prior tested image digest and
recreate the app. Startup rejects unknown migration versions. If a schema
migration was applied, assess compatibility and prefer a forward fix. Do not
delete migration history to bypass validation or restore over the live database
while customers are placing orders.

Rotate a leaked runtime password by creating a replacement credential, updating
the secret store, recreating the app, verifying readiness, and revoking the old
credential. Rotate SMTP and bootstrap tokens through the same process. Preserve
the previous image digest and the backup identifier with each release record.

## Public launch requirements

Confirm the currency, actual tax policy, shipping charges and service regions,
terms, privacy policy, returns process, backup recovery, and working email with
the business owner. The current pricing rule is USD, estimated 8% tax, and
$6.50 shipping below an $80 subtotal. The footer contains unfinished policy links.
These defaults need explicit business approval or implementation before public
trade. Durable email delivery via an outbox, vendor tenancy, online payment
providers, and admin MFA remain in the roadmap.

## Platform references

- [Go 1.26 HTTP shutdown](https://pkg.go.dev/net/http@go1.26.9#Server.Shutdown)
- [Compose health and startup ordering](https://docs.docker.com/compose/how-tos/startup-order/)
- [Caddy forwarding behavior](https://caddyserver.com/docs/caddyfile/directives/reverse_proxy)
- [PostgreSQL 17 pg_dump](https://www.postgresql.org/docs/17/app-pgdump.html)
- [PostgreSQL 17 pg_restore](https://www.postgresql.org/docs/17/app-pgrestore.html)

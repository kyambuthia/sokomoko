# Deployment readiness verification — October 17, 2026

## Result and scope

The source has a verified container deployment path for one application instance
with cash-on-delivery checkout and a trusted platform operations team. The
architecture remains a modular Go monolith with an independent release migration
task, PostgreSQL, a TLS proxy, and explicit HTTP/maintenance lifecycle ownership.

This work did not provision a live environment. External vendor accounts,
online payment providers, legal policies, and business pricing decisions remain
outside the verified launch scope. See [the runbook](../DEPLOYMENT.md) and
[the architecture decision](../DEPLOYMENT_PLAN.md).

## Verification performed

The pre-existing local PostgreSQL instance rejected the README's development
credentials. Database verification instead used a newly created PostgreSQL 17
container on `127.0.0.1:55432`, with disposable schemas and test-only credentials.

All commands below completed successfully:

```bash
SOKOMOKO_TEST_DATABASE_URL='postgres://sokomoko:sokomoko@127.0.0.1:55432/sokomoko_test?sslmode=disable' SOKOMOKO_REQUIRE_DB_TESTS=1 go test -race ./...
go test -race ./internal/lifecycle ./internal/app ./internal/notify
go vet ./...
make lint
go mod tidy
git diff --exit-code go.mod go.sum
bash -n scripts/smoke-image.sh
docker compose config --quiet
docker compose --env-file deploy/production.env.example -f deploy/compose.production.yml config --quiet
go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 -show verbose ./...
docker build --build-arg VERSION=deployment-readiness -t sokomoko:deployment-check .
bash scripts/smoke-image.sh sokomoko:deployment-check
git diff --check
```

The focused race checks were repeated after final review corrections to IPv6
host handling, SMTP validation messages, and draining on listener failure.
The image was rebuilt and the smoke check repeated after the runtime changes.
Caddy 2.11.4 also accepted `deploy/Caddyfile` using its `validate` command with
the example hostname and certificate-contact environment variables.

The full suite passed across all packages, including database concurrency,
checkout idempotency and pricing, auth, role routing, and the new payment,
configuration, schema, health probe, and lifecycle coverage. Tests require a
database in CI, so a missing database cannot silently skip the integration suite.

## Container evidence

The final local image was approximately 64.5 MB, configured as
`nonroot:nonroot`, with a direct executable health probe requiring no shell.
The smoke check demonstrated:

- Production migrations apply and can be repeated safely.
- Runtime startup works with a separate role lacking schema CREATE and
  migration-history INSERT privileges.
- The container runs with a read-only filesystem and all capabilities dropped.
- Storefront, admin, and operations hosts answer readiness probes.
- Production startup does not seed demonstration products.
- SIGTERM yields exit code zero before the database is removed.
- A custom-format backup restores into a different database in one transaction;
  migration-history and warehouse row counts match the source database.

The smoke script removes only the containers, network, and temporary backup it
creates. It is included in the Docker CI job.

## Dependency findings

The repository now builds with Go 1.26.9, `golang.org/x/crypto` v0.58.0,
`golang.org/x/text` v0.43.0, and `golang.org/x/sync` v0.24.0.
The final vulnerability scan reported **zero affected symbols and zero
vulnerabilities in imported packages**. It also reported the module-level
[OpenPGP advisory GO-2026-5932](https://pkg.go.dev/vuln/GO-2026-5932) in
`x/crypto`; this repository uses bcrypt and does not import OpenPGP. The advisory
has no fixed version. CI runs a pinned scanner weekly and on pushes and PRs.

## Operational limits

No real certificate issuance, external SMTP delivery, payment settlement, load
test, or production-data recovery drill was performed. The backup check used
the disposable migrated database; an operator must demonstrate recovery against
representative data and the agreed RPO/RTO before launch. Configure actual DNS,
database roles and credentials, email, and backup retention using the runbook.

Vendor membership and query scoping, per-partner fulfillment, shared abuse
controls, a durable email outbox, admin MFA, and request-context propagation
through commerce services remain documented architecture work. The current
tax, shipping, currency, and policy defaults require business approval or
implementation before public trade.

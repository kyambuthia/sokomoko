# Sokomoko TODO

Last updated: March 26, 2026

This file is the consolidated backlog for work that is still not implemented.
It pulls together the remaining architecture tasks from the current refactor effort plus the open production-readiness and product tasks already noted elsewhere in the docs.

## Phase 1: Architecture Stabilization

- [ ] Add focused tests for the new service adapter layers in `internal/service/*/store.go` so DB-to-domain mapping is validated directly instead of only through higher-level tests.
- [ ] Decide whether `cmd/sokomoko` should be split further so server assembly, host routing, and middleware wiring move out of the command package.
- [ ] Reduce remaining route/view-model assembly in public and auth flows where handlers still own page-shaping logic.
- [ ] Introduce a stronger request-identity abstraction at the auth middleware boundary so route code no longer depends on context lookups directly.
- [ ] Review whether any feature services still expose low-value transport or persistence details that should become local domain types.
- [ ] Add more package-level tests around app composition and command wiring beyond the current smoke coverage.

## Phase 2: Deployment Hard Blockers

- [ ] Enforce HTTPS-only deployment defaults and verify secure-cookie behavior in production.
- [ ] Normalize transient SQLite failures (`busy`, `locked`) into retryable responses and operational logs.
- [ ] Add startup environment validation with required-env checks and warnings for unsafe defaults.
- [ ] Add a backup/restore runbook and an automated SQLite backup schedule.

## Phase 3: Commerce and Checkout

- [ ] Replace placeholder payment methods with a real payment provider.
- [ ] Persist payment state transitions and provider transaction IDs.
- [ ] Add idempotency keys for checkout submissions.
- [ ] Add shipping profiles, region-aware rates, and ETA logic.
- [ ] Add a customer order detail page at `/account/orders/{id}`.
- [ ] Implement cancellation and refund policy enforcement.
- [ ] Add inventory reservation timeout and release behavior.

## Phase 4: Identity and Account Recovery

- [ ] Add login-attempt throttling and lockout behavior for auth endpoints.
- [ ] Add verified-email flow for new users.
- [ ] Add optional MFA for admin users.
- [ ] Add CSRF protection for unsafe form submissions.
- [ ] Strengthen password complexity requirements.
- [ ] Consider extended-session or “remember me” support.
- [ ] Evaluate whether OAuth/OIDC login providers are in scope.

## Phase 5: Admin, Staff, and Partner Operations

- [ ] Finish admin product CRUD flows, including create, edit, archive, and tests.
- [ ] Add staff order assignment and SLA indicators.
- [ ] Add audit-log export tooling and retention policy.
- [ ] Add partner bulk fulfillment actions.
- [ ] Add reusable delivery-notice templates for partner workflows.

## Phase 6: Observability and Operations

- [ ] Add a metrics endpoint for requests, latencies, auth failures, and checkout outcomes.
- [ ] Add structured JSON logging with request IDs and actor IDs.
- [ ] Define alerts for elevated `5xx`, login failures, and DB lock frequency.
- [ ] Add CI/CD gates for tests, linting, vulnerability scanning, and smoke tests.

## Phase 7: Launch and Compliance

- [ ] Add Terms, Privacy, and Returns policy pages plus footer links.
- [ ] Run a dependency audit and generate an SBOM.
- [ ] Add baseline load testing and response-time SLO verification.
- [ ] Complete a final accessibility pass covering keyboard flow, landmarks, contrast, and form errors.

## Phase 8: Documentation and Housekeeping

- [ ] Keep `docs/roadmap/DEPLOYMENT_READINESS_ROADMAP.md` and this file synchronized when work lands.
- [ ] Remove or update any remaining design-doc language that implies the whole product is already production-ready when that only applies to the UI system.
- [ ] Add operator-facing docs for production startup, backups, restore, and incident response.
- [ ] Add a short contributor-oriented architecture overview that explains the current composition, service, route, and DB layers.

## Notes

- Completed refactor phases already landed include: app-scoped auth, typed DB errors, DB layer split by concern, feature service extraction, service adapters, explicit startup commands, and centralized app composition.
- The most urgent unimplemented work is still in deployment hardening, payment integration, and auth/security policy.

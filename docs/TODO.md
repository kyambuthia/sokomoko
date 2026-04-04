# Sokomoko TODO

Last updated: April 4, 2026

This file is the maintained backlog for still-open engineering, product, and deployment work.

## Priority Now

These are the next items worth doing before expanding feature surface area further.

- [ ] Replace the current same-origin-only CSRF check with real CSRF protection for login, signup, password reset, logout, checkout, and admin or partner form posts.
- [ ] Add auth throttling, lockout policy, and audit logging for login and password-reset endpoints.
- [ ] Make admin bootstrap transactional so root-admin creation and staff provisioning either succeed together or roll back together.
- [ ] Add startup environment validation and fail-fast behavior for production-critical config such as SMTP, HTTPS expectations, and cookie-domain settings.
- [ ] Replace placeholder payments with a real payment flow and add idempotency keys for checkout submissions.

## Phase 1: Architecture Stabilization

- [ ] Add focused tests for the service adapter layers in `internal/service/*/store.go` so DB-to-domain mapping is validated directly instead of only through higher-level tests.
- [ ] Decide whether `cmd/sokomoko` should be split further so server assembly, host routing, and middleware wiring move out of the command package.
- [ ] Reduce remaining route or view-model assembly in public and auth flows where handlers still own page-shaping logic.
- [ ] Introduce a stronger request-identity abstraction at the auth middleware boundary so route code no longer depends on context lookups directly.
- [ ] Review whether any feature services still expose low-value transport or persistence details that should become local domain types.
- [ ] Add more package-level tests around app composition and command wiring beyond the current smoke coverage.

## Phase 2: Deployment Hard Blockers

- [ ] Enforce HTTPS-only deployment defaults and verify secure-cookie behavior in production.
- [ ] Normalize transient SQLite failures (`busy`, `locked`) into retryable responses and operational logs.
- [ ] Add startup environment validation with required-env checks and warnings for unsafe defaults.
- [ ] Fail startup when password-reset email is expected in production but SMTP configuration is incomplete or invalid.
- [ ] Add a backup or restore runbook and an automated SQLite backup schedule.

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
- [ ] Replace same-origin-only CSRF checks with token-based protection for unsafe form submissions.
- [ ] Add verified-email flow for new users.
- [ ] Add optional MFA for admin users.
- [ ] Strengthen password complexity requirements.
- [ ] Consider extended-session or "remember me" support.
- [ ] Evaluate whether OAuth or OIDC login providers are in scope.

## Phase 5: Admin, Staff, and Partner Operations

- [ ] Make the admin setup flow transactional and recoverable when staff provisioning fails.
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
- [ ] Remove or update any remaining design-doc language that implies the whole product is already production-ready when that only applies to selected subsystems.
- [ ] Add operator-facing docs for production startup, backups, restore, and incident response.
- [ ] Continue aligning onboarding docs with the current host-routed architecture rather than the older design-system walkthrough.

## Notes

- Completed refactor phases already landed include: app-scoped auth, typed DB errors, DB layer split by concern, feature service extraction, service adapters, explicit startup commands, and centralized app composition.
- The most urgent unimplemented work is still in auth hardening, startup validation, payment integration, and operational safety.
- The longer-term Go commerce evolution plan is tracked in `docs/roadmap/GO_COMMERCE_EVOLUTION_PLAN.md`.

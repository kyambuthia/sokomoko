# Deployment Readiness Roadmap

Last updated: April 4, 2026

## Goal

Ship Sokomoko to production with baseline security, reliability, observability, and complete customer, admin, staff, and partner operational flows.

## Current Priority Order

These are the most important remaining deployment items in order:

1. Replace the current same-origin-only CSRF check with real CSRF protection across auth and operational forms.
2. Add auth throttling and lockout behavior for login and password-reset flows.
3. Add startup validation and fail-fast production checks for SMTP, HTTPS, and cookie-domain configuration.
4. Make admin bootstrap transactional so partial staff provisioning cannot leave unusable or hidden credentials.
5. Replace placeholder payments and add idempotent checkout submission.

## Phase 0: Hard Blockers

- [ ] Security: Replace same-origin-only CSRF checking with token-based CSRF protection for unsafe form submissions.
- [x] Security: Add request rate limiting middleware for write surfaces.
- [x] Security: Finalize stronger input validation for account creation, admin setup, and staff creation.
- [ ] Security: Enforce HTTPS-only deployment defaults and verify secure-cookie behavior in production.
- [ ] Reliability: Normalize transient DB failures (`locked`, `busy`) into retryable responses and operational logs.
- [x] Reliability: Add production error pages for `404` and `500` with request ID exposure for support.
- [ ] Operations: Add startup environment validation and unsafe-default warnings.
- [ ] Operations: Fail fast when production password-reset email delivery is misconfigured.
- [ ] Operations: Add backup or restore runbook and automated SQLite backup schedule.

## Phase 1: Checkout and Orders

- [ ] Checkout: Integrate a real payment provider and remove placeholder payment methods.
- [ ] Checkout: Persist payment state (`authorized`, `captured`, `failed`, `refunded`) and provider transaction IDs.
- [ ] Checkout: Add idempotency keys for checkout submits.
- [ ] Checkout: Add shipping profiles or rates by region and delivery ETA logic.
- [ ] Orders: Add customer order detail page (`/account/orders/{id}`).
- [ ] Orders: Add cancellation and refund policy enforcement.
- [ ] Orders: Add inventory reservation timeout and release strategy.

## Phase 2: Identity and Account Recovery

- [ ] Auth: Add login-attempt throttling or lockout policy and audit logs for suspicious auth behavior.
- [x] Auth: Add email delivery integration for reset links.
- [x] Auth: Invalidate active sessions on password reset or change.
- [ ] Auth: Add verified-email flow for new users.
- [ ] Auth: Add optional MFA for admin users.

## Phase 3: Admin, Staff, and Partner Operations

- [ ] Admin: Make the initial admin bootstrap flow transactional and recoverable.
- [ ] Admin: Complete product CRUD routes (new, edit, archive) with validation and test coverage.
- [ ] Admin: Add order assignment and SLA indicators for staff.
- [ ] Admin: Add audit export tooling and retention policy.
- [ ] Partner: Add bulk fulfillment actions and delivery-notice templates.

## Phase 4: Observability and Operability

- [ ] Monitoring: Add metrics endpoint for requests, latencies, auth failures, and checkout outcomes.
- [ ] Logging: Add structured JSON logs with request IDs and actor IDs.
- [ ] Alerting: Create alerts for elevated `5xx`, login failures, and DB lock frequency.
- [ ] CI/CD: Add pipeline gates for tests, linting, vulnerability scanning, and smoke tests.

## Phase 5: Compliance and Launch Hygiene

- [ ] Legal: Add Terms, Privacy, and Returns policy pages and footer links.
- [ ] Security: Run a dependency audit and generate an SBOM.
- [ ] Performance: Add baseline load tests and response-time SLO verification.
- [ ] UX: Complete a final accessibility pass for keyboard navigation, landmarks, contrast, and form errors.

## Completed Foundations

- [x] Centralized app composition and explicit startup commands.
- [x] Split DB logic by concern with typed DB error helpers.
- [x] Extract feature services for account, admin, commerce, partner, and catalog flows.
- [x] Add request IDs, security headers, body limits, trusted-host validation, and panic recovery.
- [x] Add periodic cleanup for expired sessions and password-reset tokens.

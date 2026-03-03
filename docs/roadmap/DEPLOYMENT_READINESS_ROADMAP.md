# Deployment Readiness Roadmap

Last updated: March 4, 2026

## Goal
Ship Sokomoko to production with baseline security, reliability, observability, and complete customer/admin/staff operational flows.

## Phase 0: Hard Blockers (Must ship first)
- [ ] Security: Add request rate limiting for write/auth surfaces.
- [ ] Security: Finalize strong input validation for account creation/admin setup/staff creation.
- [ ] Security: Enforce HTTPS-only deployment defaults and verify secure cookie behavior in production.
- [ ] Reliability: Normalize transient DB failures (locked/busy) to retryable responses and logs.
- [ ] Reliability: Add production error pages for 404/500 with request ID exposure for support.
- [ ] Operations: Add backup/restore runbook and automated SQLite backup schedule.
- [ ] Operations: Add environment validation at startup (required env checks, unsafe default warnings).

## Phase 1: Checkout + Orders (Production commerce baseline)
- [ ] Checkout: Integrate real payment provider (replace placeholders) with success/failure webhooks.
- [ ] Checkout: Persist payment state (authorized, captured, failed, refunded) and transaction IDs.
- [ ] Checkout: Add idempotency keys for checkout submits.
- [ ] Checkout: Add shipping profiles/rates by region and delivery ETA logic.
- [ ] Orders: Add customer order detail page (`/account/orders/{id}`).
- [ ] Orders: Add cancellation/refund policy enforcement.
- [ ] Orders: Add inventory reservation timeout and release strategy.

## Phase 2: Identity + Account Recovery
- [ ] Auth: Add login attempt throttling/lockout policy and audit logs for suspicious auth behavior.
- [ ] Auth: Add email delivery integration for reset links (production-safe path).
- [ ] Auth: Add verified-email flow for new users.
- [ ] Auth: Invalidate active sessions on password reset/change.
- [ ] Auth: Add optional MFA for admin users.

## Phase 3: Admin and Staff Operations
- [ ] Admin: Complete product CRUD routes (new/edit/archive) with validation and test coverage.
- [ ] Admin: Add order assignment and SLA indicators for staff.
- [ ] Admin: Add audit export tooling and retention policy.
- [ ] Partner: Add bulk fulfillment actions and delivery-notice templates.

## Phase 4: Observability + Operability
- [ ] Monitoring: Add metrics endpoint (requests, latencies, auth failures, checkout outcomes).
- [ ] Logging: Add structured JSON logs with request IDs and actor IDs.
- [ ] Alerting: Create alerts for elevated 5xx, login failures, and DB lock frequency.
- [ ] CI/CD: Add deploy pipeline gates (tests, lint, vulnerability scan, smoke tests).

## Phase 5: Compliance and Launch Hygiene
- [ ] Legal: Add Terms, Privacy, Returns policy pages and footer links.
- [ ] Security: Dependency audit and SBOM generation.
- [ ] Performance: Baseline load tests and response-time SLO verification.
- [ ] UX: Final accessibility pass (keyboard navigation, landmarks, contrast, form errors).

## Work Started This Session
- [x] Create this roadmap and prioritize by deployment criticality.
- [ ] Implement request rate limiting with test coverage.
- [ ] Implement stronger account input validation with test coverage.
- [ ] Re-run full user/admin/staff end-to-end flows after changes.

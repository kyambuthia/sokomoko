# Roadmap

This roadmap follows the current source boundaries and the known issues in
[08-known-issues.md](08-known-issues.md).

## Phase 1: correctness and security

1. Add CSRF fields to every authenticated unsafe template.
2. Add the CSRF header to authenticated JavaScript requests.
3. Make the IP limiter honor the methods passed to it.
4. Make admin setup atomic and recoverable.
5. Add startup validation for production cookie, host, SMTP, and database
   configuration.

## Phase 2: identity and operations

1. Expand explicit authorization tests for every host and role combination.
2. Add database backup and restore procedures.
3. Add structured logs, metrics, and deployment smoke tests.

## Phase 3: commerce completeness

1. Replace placeholder card and mobile-money payment methods.
2. Model payment attempts and provider callbacks as durable state.
3. Add customer order detail, cancellation, refund, and delivery policy flows.
4. Move toward integer or decimal-safe money storage instead of SQLite REAL.
5. Add shipping and tax policies beyond the current flat estimates.

## Phase 4: repository cleanup

1. Remove the stale db/schema.sql snapshot or make it generated from the
   canonical schema.
2. Remove unused templates or register them intentionally.
3. Keep one current UI reference instead of historical design reports.
4. Add CI checks for formatting, tests, vet, and accidental generated files.

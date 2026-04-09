# Sokomoko Code Review Findings

Last updated: April 9, 2026

## Phase 1: Security Issues

- **[HIGH] Session Hash Fallback** - `internal/db/sessions.go:19-34` - `GetSession` allows both hashed and unhashed session IDs, creating confusion and potential security issue
- **[MEDIUM] Rate Limiter Bypass** - `internal/app/rate_limit.go:84-88` - Non-POST requests bypass rate limiting when method filtering is enabled
- **[LOW] Admin Setup Token Timing** - `internal/auth/auth.go:282-285` - Length check done before constant-time comparison
- **[LOW] Rate Limiter Memory Growth** - `internal/app/rate_limit.go:57-66` - Cleanup only triggers at 4096 entries, allowing memory growth under attack

## Phase 2: Code Quality

- **[MEDIUM] Float for Money** - Schema uses REAL for prices/amounts (lines 31, 156 in `db/schema.sql`) - should use INTEGER (cents) to avoid floating-point precision issues
- **[LOW] Missing Database Indexes** - No index on `sessions.user_id` for session lookup by user; no index on `password_reset_tokens.token_hash`

## Phase 3: Race Conditions

- **[HIGH] Cart Stock Race Condition** - `internal/service/commerce/commerce.go:40-74` - `AddToCart` reads stock, checks, then writes without atomic locking
- **[HIGH] Checkout Stock Reservation** - `internal/service/checkout/checkout.go:155-210` - Stock can change between reservation and order placement
- **[MEDIUM] Session Cleanup Timing** - `cmd/sokomoko/server.go:114-137` - Background cleanup runs every 15 minutes; expired sessions may linger

## Phase 4: Bugs

- **[MEDIUM] Inventory Stock Sync Trigger** - `db/schema.sql:268-270` - INSERT OR IGNORE only runs during schema creation, new products won't get inventory stock records
- **[LOW] Empty Cart Edge Case** - `internal/db/cart_orders.go:272-279` - Error handling doesn't cover all paths where cart could become empty between check and order creation

## Phase 5: Architectural Issues

- **[MEDIUM] No Database Migrations** - Schema version management missing; no incremental migration system
- **[LOW] Inconsistent Service Store Pattern** - Some services have `store.go` (commerce, checkout, partner) while others don't

## Phase 6: Edge Cases

- **[LOW] Concurrent Password Resets** - Multiple reset requests for same user create multiple valid tokens (no token limit per user)
- **[LOW] Session Fixation After Password Change** - Sessions only deleted on password reset via `UsePasswordResetToken`; direct password updates leave sessions intact
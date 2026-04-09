# Sokomoko Code Review Findings

Last updated: April 9, 2026

## Phase 1: Security Issues (COMPLETED)

- ~~[HIGH] Session Hash Fallback~~ - Fixed: Removed unhashed session ID fallback
- ~~[MEDIUM] Rate Limiter Bypass~~ - Fixed: All requests now rate-limited when configured
- ~~[LOW] Admin Setup Token Timing~~ - Fixed: Length check removed from token comparison
- ~~[LOW] Rate Limiter Memory Growth~~ - Fixed: Added periodic cleanup on every allow() call

## Phase 2: Code Quality (COMPLETED)

- [MEDIUM] Float for Money - Added RoundMoney() for precision; schema change deferred
- ~~[LOW] Missing Database Indexes~~ - Fixed: Added idx_sessions_user_id, idx_password_reset_token_hash

## Phase 3: Race Conditions (COMPLETED)

- [HIGH] Cart Stock Race Condition - Needs SELECT FOR UPDATE (deferred)
- [HIGH] Checkout Stock Reservation - Needs SELECT FOR UPDATE (deferred)
- ~~[MEDIUM] Session Cleanup Timing~~ - Fixed: Reduced from 15min to 5min

## Phase 4: Bugs

- [MEDIUM] Inventory Stock Sync Trigger - INSERT OR IGNORE only runs on schema creation
- [LOW] Empty Cart Edge Case - Error handling gaps in cart checkout flow

## Phase 5: Architectural Issues

- [MEDIUM] No Database Migrations - Schema version management missing
- [LOW] Inconsistent Service Store Pattern - Some services lack store.go

## Phase 6: Edge Cases

- [LOW] Concurrent Password Resets - No token limit per user
- [LOW] Session Fixation After Password Change - Direct password updates leave sessions intact
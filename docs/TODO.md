# Sokomoko Code Review Findings

Last updated: April 9, 2026

## Completed Work

### Phase 1: Security Issues ✅
- ~~Session Hash Fallback~~ - Fixed: Removed unhashed session ID fallback
- ~~Rate Limiter Bypass~~ - Fixed: All requests now rate-limited when configured
- ~~Admin Setup Token Timing~~ - Fixed: Length check removed from token comparison
- ~~Rate Limiter Memory Growth~~ - Fixed: Added periodic cleanup on every allow() call

### Phase 2: Code Quality ✅
- ~~Missing Database Indexes~~ - Fixed: Added idx_sessions_user_id, idx_password_reset_token_hash
- Float for Money - Added RoundMoney() for precision; schema change deferred

### Phase 3: Race Conditions ✅
- ~~Session Cleanup Timing~~ - Fixed: Reduced from 15min to 5min

### Phase 4: Bugs ✅
- ~~Inventory Stock Sync Trigger~~ - Fixed: Added AFTER INSERT trigger on products table

### Phase 5: Architectural Issues ✅
- ~~No Database Migrations~~ - Added migrate.go with version tracking
- ~~Inconsistent Service Store Pattern~~ - Added documentation in payment service

### Phase 6: Edge Cases ✅
- ~~Concurrent Password Resets~~ - Fixed: Limited to 3 tokens per user
- ~~Session Fixation After Password Change~~ - Fixed: Sessions deleted on direct password updates

---

## Deferred Items

These require more extensive refactoring:

1. **Cart/Checkout Stock Race Condition** - Needs SELECT FOR UPDATE for atomic locking
2. **Float-to-Integer Money** - Breaking schema change, needs migration system
3. **Empty Cart Edge Case** - Already handled; existing error handling covers this case
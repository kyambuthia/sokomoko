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

### Phase 5: Architectural Issues ✅
- ~~No Database Migrations~~ - Added migrate.go with version tracking
- ~~Inconsistent Service Store Pattern~~ - Added documentation in payment service

### Phase 6: Edge Cases ✅
- ~~Concurrent Password Resets~~ - Fixed: Limited to 3 tokens per user
- ~~Session Fixation After Password Change~~ - Fixed: Sessions deleted on direct password updates

---

## Remaining Tasks (In Progress)

### Task A: Cart Stock Race Condition
- **Priority**: HIGH
- **Location**: `internal/service/commerce/commerce.go`, `internal/db/inventory.go`
- **Issue**: AddToCart reads stock, checks, then writes without atomic locking
- **Fix**: Use SELECT FOR UPDATE in the stock reservation queries to prevent race conditions

### Task B: Checkout Stock Reservation
- **Priority**: HIGH
- **Location**: `internal/service/checkout/checkout.go`, `internal/db/inventory.go`
- **Issue**: Stock can change between reservation and order placement
- **Fix**: Ensure proper locking and re-validation in the reservation flow

### Task C: Inventory Stock Sync Trigger
- **Priority**: MEDIUM
- **Location**: `db/schema.sql`, `internal/db/catalog.go`
- **Issue**: INSERT OR IGNORE only runs during schema creation, new products won't get inventory stock records
- **Fix**: Add a trigger or ensure products get inventory stock records created when added

### Task D: Empty Cart Edge Case
- **Priority**: LOW
- **Location**: `internal/db/cart_orders.go`, `internal/routes/commerce.go`
- **Issue**: Error handling doesn't cover all paths where cart could become empty between check and order creation
- **Fix**: Improve error handling in cart checkout flow
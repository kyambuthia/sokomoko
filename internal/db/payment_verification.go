package db

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// TryAcquirePaymentVerification allows one replica to check an attempt only
// after the shared verification interval has elapsed.
func (s *Store) TryAcquirePaymentVerification(ctx context.Context, attemptID int, interval time.Duration) (bool, error) {
	if attemptID <= 0 || interval <= 0 || interval.Microseconds() <= 0 {
		return false, errors.New("invalid payment verification throttle")
	}
	ctx, cancel := s.queryContext(ctx)
	defer cancel()

	var claimedID int
	err := s.DB.QueryRowContext(ctx, `
		UPDATE payment_attempts
		SET last_verification_at = statement_timestamp()
		WHERE id = $1 AND status IN ('pending','failed')
		  AND (last_verification_at IS NULL OR last_verification_at <=
		       statement_timestamp() - $2::double precision * interval '1 microsecond')
		RETURNING id`, attemptID, interval.Microseconds()).Scan(&claimedID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

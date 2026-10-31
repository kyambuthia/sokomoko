package db

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"strings"
)

// ServiceBookingByIdempotency looks up a committed request before the caller
// reads mutable listing details, preserving retries after a listing edit.
func (s *Store) ServiceBookingByIdempotency(ctx context.Context, customerID int, key string, requestHash []byte) (*ServiceBooking, error) {
	if customerID <= 0 || key != strings.TrimSpace(key) || len(key) < 16 || len(key) > 128 || len(requestHash) != 32 {
		return nil, ErrServiceBookingInvalidRequest
	}
	ctx, cancel := s.queryContext(ctx)
	defer cancel()

	var userID int
	err := s.DB.QueryRowContext(ctx,
		`SELECT id FROM users WHERE id=$1 AND role=$2 AND deleted_at IS NULL`,
		customerID, RoleUser,
	).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrServiceBookingForbidden
	}
	if err != nil {
		return nil, err
	}

	booking, err := scanServiceBooking(s.DB.QueryRowContext(ctx,
		`SELECT `+serviceBookingColumns+` FROM service_bookings WHERE customer_id=$1 AND idempotency_key=$2`,
		customerID, key,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(booking.RequestHash, requestHash) {
		return nil, ErrServiceBookingIdempotencyConflict
	}
	return booking, nil
}

package db

import (
	"context"
	"time"
)

type ServiceBookingInterval struct {
	StartsAt time.Time
	EndsAt   time.Time
}

// ServiceBookingIntervals returns occupied intervals for a service within one
// local-day window. Overdue requests do not hide otherwise open times.
func (s *Store) ServiceBookingIntervals(ctx context.Context, listingID int, from, to time.Time) ([]ServiceBookingInterval, error) {
	if listingID <= 0 || !to.After(from) {
		return nil, ErrServiceBookingInvalidRequest
	}
	ctx, cancel := s.queryContext(ctx)
	defer cancel()

	rows, err := s.DB.QueryContext(ctx,
		"SELECT starts_at,ends_at FROM service_bookings "+
			"WHERE service_listing_id=$1 AND starts_at < $3 AND ends_at > $2 "+
			"AND (status IN ('confirmed','in_progress') OR "+
			"(status='requested' AND response_deadline > statement_timestamp())) "+
			"ORDER BY starts_at,id",
		listingID, from.UTC(), to.UTC(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	intervals := make([]ServiceBookingInterval, 0)
	for rows.Next() {
		var interval ServiceBookingInterval
		if err := rows.Scan(&interval.StartsAt, &interval.EndsAt); err != nil {
			return nil, err
		}
		intervals = append(intervals, interval)
	}
	return intervals, rows.Err()
}

package db

import (
	"context"
	"database/sql"
)

// ExpireServiceBookingRequests expires timed-out partner response holds and
// writes matching system events in the same transaction.
func (s *Store) ExpireServiceBookingRequests(ctx context.Context) (int, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()

	expiredCount := 0
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		expiredCount = 0
		rows, err := tx.QueryContext(ctx, `
			WITH expired_rows AS MATERIALIZED (
			    SELECT id FROM service_bookings
			    WHERE status='requested' AND response_deadline <= statement_timestamp()
			    ORDER BY response_deadline,id
			    LIMIT 500 FOR UPDATE SKIP LOCKED
			),
			expired AS (
			    UPDATE service_bookings b
			    SET status='expired',version=b.version+1
			    FROM expired_rows e WHERE b.id=e.id
			    RETURNING b.id
			)
			SELECT id FROM expired ORDER BY id`)
		if err != nil {
			return err
		}
		ids := make([]int64, 0)
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
		for _, id := range ids {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO service_booking_events(booking_id,actor_user_id,from_status,to_status)
				 VALUES($1,NULL,'requested','expired')`,
				id,
			); err != nil {
				return err
			}
		}
		expiredCount = len(ids)
		return nil
	})
	return expiredCount, err
}

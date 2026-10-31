package db

import (
	"context"
	"database/sql"
	"errors"
)

var (
	ErrServiceBookingNotFound        = errors.New("service booking not found")
	ErrServiceBookingInvalidState    = errors.New("service booking cannot move to that state")
	ErrServiceBookingVersionConflict = errors.New("service booking changed; reload before updating")
	ErrServiceBookingResponseExpired = errors.New("provider response deadline has passed")
)

const serviceBookingQualifiedColumns = `b.id,b.service_listing_id,b.resource_id,b.customer_id,b.service_title,b.service_area,
	b.delivery_mode,b.duration_minutes,b.price_mode,b.price_cents,b.currency,b.timezone,b.starts_at,b.ends_at,b.status,
	b.response_deadline,b.version,b.created_at,b.updated_at,b.request_hash`

func validBookingPage(userID, limit, offset int) bool {
	return userID > 0 && limit >= 1 && limit <= 50 && offset >= 0 && offset <= 50000
}

// ServiceBookingsForCustomer returns a customer's own appointment requests.
func (s *Store) ServiceBookingsForCustomer(ctx context.Context, customerID, limit, offset int) ([]ServiceBooking, int, error) {
	if !validBookingPage(customerID, limit, offset) {
		return nil, 0, ErrServiceBookingInvalidRequest
	}
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback()

	var authorizedUserID int
	err = tx.QueryRowContext(ctx,
		`SELECT id FROM users WHERE id=$1 AND role=$2 AND deleted_at IS NULL`,
		customerID, RoleUser,
	).Scan(&authorizedUserID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, 0, ErrServiceBookingForbidden
	}
	if err != nil {
		return nil, 0, err
	}

	var total int
	if err := tx.QueryRowContext(ctx,
		`SELECT count(*) FROM service_bookings WHERE customer_id=$1`,
		customerID,
	).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := tx.QueryContext(ctx,
		`SELECT `+serviceBookingColumns+`
		 FROM service_bookings WHERE customer_id=$1
		 ORDER BY created_at DESC,id DESC LIMIT $2 OFFSET $3`,
		customerID, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	items := make([]ServiceBooking, 0)
	for rows.Next() {
		booking, err := scanServiceBooking(rows)
		if err != nil {
			rows.Close()
			return nil, 0, err
		}
		items = append(items, *booking)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, 0, err
	}
	if err := rows.Close(); err != nil {
		return nil, 0, err
	}
	if err := tx.Commit(); err != nil {
		return nil, 0, wrapDBError(err)
	}
	return items, total, nil
}

// ServiceBookingsForPartner scopes the queue through active partner membership.
func (s *Store) ServiceBookingsForPartner(ctx context.Context, partnerUserID, limit, offset int) ([]ServiceBooking, int, error) {
	if !validBookingPage(partnerUserID, limit, offset) {
		return nil, 0, ErrServiceBookingInvalidRequest
	}
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback()

	var allowed bool
	err = tx.QueryRowContext(ctx, `
		SELECT EXISTS (
		    SELECT 1 FROM users u
		    JOIN partner_users pu ON pu.user_id=u.id
		    JOIN partners p ON p.id=pu.partner_id
		    WHERE u.id=$1 AND u.role=$2 AND u.deleted_at IS NULL AND p.status='active'
		)`,
		partnerUserID, RolePartner,
	).Scan(&allowed)
	if err != nil {
		return nil, 0, err
	}
	if !allowed {
		return nil, 0, ErrServiceBookingForbidden
	}

	scope := `FROM service_bookings b
		JOIN service_listings s ON s.id=b.service_listing_id
		JOIN partners p ON p.id=s.partner_id
		JOIN partner_users pu ON pu.partner_id=p.id
		JOIN users u ON u.id=pu.user_id
		WHERE u.id=$1 AND u.role=$2 AND u.deleted_at IS NULL AND p.status='active'`
	var total int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) `+scope, partnerUserID, RolePartner).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := tx.QueryContext(ctx,
		`SELECT `+serviceBookingQualifiedColumns+` `+scope+`
		 ORDER BY CASE b.status WHEN 'in_progress' THEN 0 WHEN 'requested' THEN 1 WHEN 'confirmed' THEN 2 ELSE 3 END,
		          CASE WHEN b.status='in_progress' THEN b.starts_at END ASC,
		          CASE WHEN b.status='requested' THEN b.response_deadline END ASC,
		          CASE WHEN b.status='confirmed' THEN b.starts_at END ASC,
		          CASE WHEN b.status NOT IN ('in_progress','requested','confirmed') THEN b.updated_at END DESC,
		          b.id DESC LIMIT $3 OFFSET $4`,
		partnerUserID, RolePartner, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	items := make([]ServiceBooking, 0)
	for rows.Next() {
		booking, err := scanServiceBooking(rows)
		if err != nil {
			rows.Close()
			return nil, 0, err
		}
		items = append(items, *booking)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, 0, err
	}
	if err := rows.Close(); err != nil {
		return nil, 0, err
	}
	if err := tx.Commit(); err != nil {
		return nil, 0, wrapDBError(err)
	}
	return items, total, nil
}

// TransitionServiceBooking permits a customer to cancel their own future
// appointment and an authorized service partner to respond to a request.
func (s *Store) TransitionServiceBooking(ctx context.Context, actorID, bookingID int, role, newStatus string, expectedVersion int64) error {
	if actorID <= 0 || bookingID <= 0 || expectedVersion <= 0 ||
		(role != RoleUser && role != RolePartner) {
		return ErrServiceBookingInvalidRequest
	}
	if role == RoleUser && newStatus != "cancelled" {
		return ErrServiceBookingForbidden
	}
	if role == RolePartner && newStatus != "confirmed" && newStatus != "rejected" && newStatus != "cancelled" &&
		newStatus != "in_progress" && newStatus != "completed" && newStatus != "no_show" {
		return ErrServiceBookingForbidden
	}

	ctx, cancel := s.queryContext(ctx)
	defer cancel()

	return s.withTx(ctx, func(tx *sql.Tx) error {
		var actorUserID int
		userQuery := `SELECT id FROM users WHERE id=$1 AND role=$2 AND deleted_at IS NULL`
		if role == RoleUser {
			userQuery += ` FOR SHARE`
		}
		err := tx.QueryRowContext(ctx, userQuery, actorID, role).Scan(&actorUserID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrServiceBookingForbidden
		}
		if err != nil {
			return err
		}

		var listingID int
		err = tx.QueryRowContext(ctx,
			`SELECT service_listing_id FROM service_bookings WHERE id=$1`,
			bookingID,
		).Scan(&listingID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrServiceBookingNotFound
		}
		if err != nil {
			return err
		}

		if role == RolePartner {
			var partnerID int
			err = tx.QueryRowContext(ctx, `
				SELECT p.id FROM partners p
				JOIN partner_users pu ON pu.partner_id=p.id
				JOIN users u ON u.id=pu.user_id
				JOIN service_listings l ON l.partner_id=p.id
				WHERE l.id=$1 AND u.id=$2 AND u.role=$3
				  AND u.deleted_at IS NULL AND p.status='active'
				FOR SHARE OF p,pu,u,l`,
				listingID, actorID, RolePartner,
			).Scan(&partnerID)
			if errors.Is(err, sql.ErrNoRows) {
				return ErrServiceBookingNotFound
			}
			if err != nil {
				return err
			}
		}

		var current struct {
			status  string
			version int64
		}
		query := `SELECT status,version FROM service_bookings WHERE id=$1`
		args := []any{bookingID}
		if role == RoleUser {
			query += ` AND customer_id=$2`
			args = append(args, actorID)
		} else {
			query += ` AND service_listing_id=$2`
			args = append(args, listingID)
		}
		query += ` FOR UPDATE`
		err = tx.QueryRowContext(ctx, query, args...).Scan(&current.status, &current.version)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrServiceBookingNotFound
		}
		if err != nil {
			return err
		}
		if current.version != expectedVersion {
			return ErrServiceBookingVersionConflict
		}

		var allowed bool
		switch {
		case role == RoleUser && newStatus == "cancelled" &&
			(current.status == "requested" || current.status == "confirmed"):
			allowed = true
		case role == RolePartner && (newStatus == "confirmed" || newStatus == "rejected") &&
			current.status == "requested":
			allowed = true
		case role == RolePartner && newStatus == "in_progress" && current.status == "confirmed":
			allowed = true
		case role == RolePartner && newStatus == "completed" && current.status == "in_progress":
			allowed = true
		case role == RolePartner && newStatus == "no_show" && current.status == "confirmed":
			allowed = true
		case role == RolePartner && newStatus == "cancelled" && current.status == "confirmed":
			allowed = true
		}
		if !allowed {
			return ErrServiceBookingInvalidState
		}

		transitionTimePredicate := ""
		switch {
		case role == RolePartner && (newStatus == "confirmed" || newStatus == "rejected"):
			transitionTimePredicate = " AND response_deadline > statement_timestamp()"
		case newStatus == "cancelled":
			transitionTimePredicate = " AND starts_at > statement_timestamp()"
		case newStatus == "in_progress":
			transitionTimePredicate = " AND starts_at <= statement_timestamp() AND ends_at > statement_timestamp()"
		case newStatus == "completed" || newStatus == "no_show":
			transitionTimePredicate = " AND ends_at <= statement_timestamp()"
		}
		var updatedVersion int64
		err = tx.QueryRowContext(ctx,
			`UPDATE service_bookings SET status=$1,version=version+1 WHERE id=$2 AND version=$3 AND status=$4`+transitionTimePredicate+`
			 RETURNING version`,
			newStatus, bookingID, expectedVersion, current.status,
		).Scan(&updatedVersion)
		if errors.Is(err, sql.ErrNoRows) {
			if role == RolePartner && (newStatus == "confirmed" || newStatus == "rejected") {
				return ErrServiceBookingResponseExpired
			}
			return ErrServiceBookingInvalidState
		}
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx,
			`INSERT INTO service_booking_events(booking_id,actor_user_id,from_status,to_status)
			 VALUES($1,$2,$3,$4)`,
			bookingID, actorID, current.status, newStatus,
		)
		return err
	})
}

package db

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kyambuthia/sokomoko/internal/money"
)

var (
	ErrServiceBookingForbidden           = errors.New("service booking is not available to this account")
	ErrServiceBookingUnavailable         = errors.New("service is not available at that time")
	ErrServiceBookingConflict            = errors.New("service time has already been requested")
	ErrServiceBookingIdempotencyConflict = errors.New("idempotency key was already used for another booking request")
	ErrServiceBookingInvalidRequest      = errors.New("invalid service booking request")
)

type ServiceBooking struct {
	ID               int64       `json:"id"`
	ServiceListingID int         `json:"service_listing_id"`
	ResourceID       int64       `json:"resource_id"`
	CustomerID       int         `json:"customer_id"`
	ServiceTitle     string      `json:"service_title"`
	ServiceArea      string      `json:"service_area"`
	DeliveryMode     string      `json:"delivery_mode"`
	DurationMinutes  int         `json:"duration_minutes"`
	PriceMode        string      `json:"price_mode"`
	PriceCents       money.Cents `json:"price_cents"`
	Currency         string      `json:"currency"`
	Timezone         string      `json:"timezone"`
	StartsAt         time.Time   `json:"starts_at"`
	EndsAt           time.Time   `json:"ends_at"`
	Status           string      `json:"status"`
	ResponseDeadline time.Time   `json:"response_deadline"`
	Version          int64       `json:"version"`
	CreatedAt        time.Time   `json:"created_at"`
	UpdatedAt        time.Time   `json:"updated_at"`
	RequestHash      []byte      `json:"-"`
}

const serviceBookingColumns = `id,service_listing_id,resource_id,customer_id,service_title,service_area,
	delivery_mode,duration_minutes,price_mode,price_cents,currency,timezone,starts_at,ends_at,status,
	response_deadline,version,created_at,updated_at,request_hash`

func scanServiceBooking(row interface{ Scan(...any) error }) (*ServiceBooking, error) {
	booking := &ServiceBooking{}
	err := row.Scan(
		&booking.ID, &booking.ServiceListingID, &booking.ResourceID, &booking.CustomerID,
		&booking.ServiceTitle, &booking.ServiceArea, &booking.DeliveryMode, &booking.DurationMinutes,
		&booking.PriceMode, &booking.PriceCents, &booking.Currency, &booking.Timezone,
		&booking.StartsAt, &booking.EndsAt, &booking.Status, &booking.ResponseDeadline,
		&booking.Version, &booking.CreatedAt, &booking.UpdatedAt, &booking.RequestHash,
	)
	if err != nil {
		return nil, err
	}
	return booking, nil
}

func serviceBookingByKeyTx(ctx context.Context, tx *sql.Tx, customerID int, key string) (*ServiceBooking, error) {
	return scanServiceBooking(tx.QueryRowContext(ctx,
		`SELECT `+serviceBookingColumns+` FROM service_bookings WHERE customer_id=$1 AND idempotency_key=$2`,
		customerID, key,
	))
}

// RequestServiceBooking reserves one resource for a local-time slot. Listing
// schedule and price snapshots are read while the listing is locked.
func (s *Store) RequestServiceBooking(ctx context.Context, customerID, listingID, expectedListingVersion int, startsAtUTC time.Time, idempotencyKey string, requestHash []byte) (*ServiceBooking, error) {
	key := strings.TrimSpace(idempotencyKey)
	if customerID <= 0 || listingID <= 0 || expectedListingVersion <= 0 ||
		key != idempotencyKey || len(key) < 16 || len(key) > 128 ||
		len(requestHash) != 32 || startsAtUTC.IsZero() ||
		startsAtUTC.Second() != 0 || startsAtUTC.Nanosecond() != 0 {
		return nil, ErrServiceBookingInvalidRequest
	}
	startsAtUTC = startsAtUTC.UTC()
	requestHash = append([]byte(nil), requestHash...)

	ctx, cancel := s.queryContext(ctx)
	defer cancel()

	var booking *ServiceBooking
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		booking = nil
		created := false
		var authorizedUserID int
		err := tx.QueryRowContext(ctx,
			`SELECT id FROM users WHERE id=$1 AND role=$2 AND deleted_at IS NULL FOR SHARE`,
			customerID, RoleUser,
		).Scan(&authorizedUserID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrServiceBookingForbidden
		}
		if err != nil {
			return err
		}

		// Replay before reading mutable listing details so retries remain stable
		// if the listing is edited or archived after the first request.
		existing, err := serviceBookingByKeyTx(ctx, tx, customerID, key)
		if err == nil {
			if !bytes.Equal(existing.RequestHash, requestHash) {
				return ErrServiceBookingIdempotencyConflict
			}
			booking = existing
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}

		listing := struct {
			title           string
			area            string
			deliveryMode    string
			durationMinutes int
			priceMode       string
			priceCents      money.Cents
			currency        string
			timezone        string
			version         int
		}{}
		err = tx.QueryRowContext(ctx,
			`SELECT s.title,s.public_area,s.delivery_mode,s.duration_minutes,
			        s.price_mode,s.price_cents,s.currency,s.timezone,s.version
			 FROM service_listings s JOIN partners p ON p.id=s.partner_id
			 WHERE s.id=$1 AND s.state='published' AND p.status='active'
			 FOR SHARE OF s,p`,
			listingID,
		).Scan(
			&listing.title, &listing.area, &listing.deliveryMode, &listing.durationMinutes,
			&listing.priceMode, &listing.priceCents, &listing.currency, &listing.timezone, &listing.version,
		)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrServiceNotFound
		}
		if err != nil {
			return err
		}
		if listing.version != expectedListingVersion {
			return ErrServiceConflict
		}

		var resourceID int64
		err = tx.QueryRowContext(ctx,
			`SELECT id FROM service_resources
			 WHERE service_listing_id=$1 AND active ORDER BY id LIMIT 1 FOR SHARE`,
			listingID,
		).Scan(&resourceID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrServiceBookingUnavailable
		}
		if err != nil {
			return err
		}

		var slotAvailable bool
		err = tx.QueryRowContext(ctx, `
			SELECT $1::timestamptz > statement_timestamp()
			   AND (($1::timestamptz AT TIME ZONE $2::text)::date =
			        (($1::timestamptz + $3::integer * interval '1 minute') AT TIME ZONE $2::text)::date)
			   AND EXTRACT(EPOCH FROM
			       (($1::timestamptz AT TIME ZONE 'UTC') - ($1::timestamptz AT TIME ZONE $2::text))
			   ) = EXTRACT(EPOCH FROM
			       ((($1::timestamptz + $3::integer * interval '1 minute') AT TIME ZONE 'UTC') -
			        (($1::timestamptz + $3::integer * interval '1 minute') AT TIME ZONE $2::text))
			   )
			   AND EXISTS (
			       SELECT 1 FROM service_weekly_hours h
			       WHERE h.service_listing_id=$4
			         AND h.weekday=EXTRACT(DOW FROM ($1::timestamptz AT TIME ZONE $2::text))::smallint
			         AND ($1::timestamptz AT TIME ZONE $2::text)::time >= h.opens_at
			         AND (($1::timestamptz + $3::integer * interval '1 minute') AT TIME ZONE $2::text)::time <= h.closes_at
			   )`,
			startsAtUTC, listing.timezone, listing.durationMinutes, listingID,
		).Scan(&slotAvailable)
		if err != nil {
			return err
		}
		if !slotAvailable {
			return ErrServiceBookingUnavailable
		}

		// Expire only overdue requests blocking this interval, locking in a
		// stable order to reduce deadlocks between requests for one resource.
		rows, err := tx.QueryContext(ctx, `
			WITH expired_rows AS MATERIALIZED (
			    SELECT id FROM service_bookings
			    WHERE resource_id=$1 AND status='requested'
			      AND response_deadline <= statement_timestamp()
			      AND time_range && tstzrange($2::timestamptz,
			          $2::timestamptz + $3::integer * interval '1 minute','[)')
			    ORDER BY id FOR UPDATE
			),
			expired AS (
			    UPDATE service_bookings b
			    SET status='expired', version=b.version+1
			    FROM expired_rows e WHERE b.id=e.id
			    RETURNING b.id
			)
			SELECT id FROM expired ORDER BY id`,
			resourceID, startsAtUTC, listing.durationMinutes,
		)
		if err != nil {
			return err
		}
		expiredIDs := make([]int64, 0)
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			expiredIDs = append(expiredIDs, id)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
		for _, id := range expiredIDs {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO service_booking_events(booking_id,actor_user_id,from_status,to_status)
				 VALUES($1,NULL,'requested','expired')`,
				id,
			); err != nil {
				return err
			}
		}

		booking, err = scanServiceBooking(tx.QueryRowContext(ctx,
			`INSERT INTO service_bookings(
			     service_listing_id,resource_id,customer_id,service_title,service_area,delivery_mode,
			     duration_minutes,price_mode,price_cents,currency,timezone,starts_at,ends_at,
			     response_deadline,idempotency_key,request_hash
			 )
			 SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,
			        $12::timestamptz + $7::integer * interval '1 minute',
			        LEAST(statement_timestamp()+interval '24 hours',$12::timestamptz),$13,$14
			 WHERE $12::timestamptz > statement_timestamp()
			 ON CONFLICT (customer_id,idempotency_key) DO NOTHING
			 RETURNING `+serviceBookingColumns,
			listingID, resourceID, customerID, listing.title, listing.area, listing.deliveryMode,
			listing.durationMinutes, listing.priceMode, listing.priceCents, listing.currency,
			listing.timezone, startsAtUTC, key, requestHash,
		))
		if errors.Is(err, sql.ErrNoRows) {
			existing, lookupErr := serviceBookingByKeyTx(ctx, tx, customerID, key)
			if lookupErr == nil {
				if !bytes.Equal(existing.RequestHash, requestHash) {
					return ErrServiceBookingIdempotencyConflict
				}
				booking = existing
				return nil
			}
			if lookupErr != nil && !errors.Is(lookupErr, sql.ErrNoRows) {
				return lookupErr
			}
			return ErrServiceBookingUnavailable
		}
		if err != nil {
			return err
		}
		created = true
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO service_booking_events(booking_id,actor_user_id,from_status,to_status)
			 VALUES($1,$2,NULL,'requested')`,
			booking.ID, customerID,
		); err != nil {
			return err
		}
		if created {
			var stillFuture bool
			if err := tx.QueryRowContext(ctx,
				`SELECT starts_at > clock_timestamp() FROM service_bookings WHERE id=$1`,
				booking.ID,
			).Scan(&stillFuture); err != nil {
				return err
			}
			if !stillFuture {
				return ErrServiceBookingUnavailable
			}
		}
		return nil
	})
	if pgCode(err) == "23P01" {
		return nil, fmt.Errorf("%w: %w", ErrServiceBookingConflict, err)
	}
	if err != nil {
		return nil, err
	}
	return booking, nil
}

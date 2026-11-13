package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"

	"github.com/kyambuthia/sokomoko/internal/domain"
)

var (
	ErrServiceNotFound = domain.ErrServiceNotFound
	ErrServiceConflict = domain.ErrServiceConflict
)

type (
	ServiceListing    = domain.ServiceListing
	ServiceWeeklyHour = domain.ServiceWeeklyHour
	ServiceFilter     = domain.ServiceFilter
)

const serviceColumns = `s.id,s.partner_id,p.name,p.slug,s.slug,s.title,s.description,s.category,s.duration_minutes,s.price_mode,s.price_cents,s.currency,s.public_area,s.delivery_mode,s.state,s.version,s.created_at,s.updated_at,s.timezone,(SELECT count(*)::integer FROM service_weekly_hours h WHERE h.service_listing_id=s.id),s.public_lat,s.public_lng`

func scanService(row interface{ Scan(...any) error }) (*ServiceListing, error) {
	v := &ServiceListing{}
	var publicLat, publicLng sql.NullFloat64
	err := row.Scan(&v.ID, &v.PartnerID, &v.PartnerName, &v.PartnerSlug, &v.Slug, &v.Title, &v.Description, &v.Category, &v.DurationMinutes, &v.PriceMode, &v.PriceCents, &v.Currency, &v.PublicArea, &v.DeliveryMode, &v.State, &v.Version, &v.CreatedAt, &v.UpdatedAt, &v.Timezone, &v.OpenDays, &publicLat, &publicLng)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrServiceNotFound
	}
	if err == nil && v.DeliveryMode == "premises" && publicLat.Valid && publicLng.Valid {
		lat, lng := publicLat.Float64, publicLng.Float64
		v.PublicLat = &lat
		v.PublicLng = &lng
	}
	return v, err
}

type serviceHourQuerier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func loadServiceWeeklyHours(ctx context.Context, q serviceHourQuerier, listingID int) ([]ServiceWeeklyHour, error) {
	rows, err := q.QueryContext(ctx, `SELECT weekday,to_char(opens_at,'HH24:MI'),to_char(closes_at,'HH24:MI') FROM service_weekly_hours WHERE service_listing_id=$1 ORDER BY weekday`, listingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	hours := []ServiceWeeklyHour{}
	for rows.Next() {
		var hour ServiceWeeklyHour
		if err := rows.Scan(&hour.Weekday, &hour.OpensAt, &hour.ClosesAt); err != nil {
			return nil, err
		}
		hours = append(hours, hour)
	}
	return hours, rows.Err()
}

// BrowseServices exposes only intentionally published offerings of active shops.
// STRPOS treats %, _, and user punctuation literally rather than as patterns.
func (s *Store) BrowseServices(ctx context.Context, f ServiceFilter) ([]ServiceListing, int, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	if f.PartnerID < 0 || f.PartnerID > 2147483647 || f.Limit < 1 || f.Limit > 50 || f.Offset < 0 || f.Offset > 50000 {
		return nil, 0, ErrServiceNotFound
	}
	predicate := `s.state='published' AND p.status='active' AND ($1='' OR strpos(lower(s.title||' '||s.description||' '||p.name),lower($1))>0) AND ($2='' OR s.category=$2) AND ($3='' OR strpos(lower(s.public_area),lower($3))>0) AND ($4='' OR s.delivery_mode=$4) AND ($5=0 OR s.partner_id=$5) AND (NOT $6 OR EXISTS(SELECT 1 FROM service_weekly_hours h WHERE h.service_listing_id=s.id))`
	args := []any{f.Query, f.Category, f.Area, f.DeliveryMode, f.PartnerID, f.HasHours}
	orderBy := ` ORDER BY lower(s.title),s.id`
	hasRadius := f.Lat != nil && f.Lng != nil && f.RadiusKM > 0
	if hasRadius {
		lat := *f.Lat
		lng := *f.Lng
		latDelta := f.RadiusKM / 111.0
		cosLat := math.Max(math.Cos(lat*math.Pi/180.0), 0.1)
		lngDelta := f.RadiusKM / (111.0 * cosLat)
		const haversineExpr = `(6371.0 * acos(LEAST(1.0, GREATEST(-1.0, cos(radians($7)) * cos(radians(s.public_lat)) * cos(radians(s.public_lng) - radians($8)) + sin(radians($7)) * sin(radians(s.public_lat))))))`
		predicate += fmt.Sprintf(
			` AND s.delivery_mode='premises' AND s.public_lat BETWEEN $9 AND $10 AND s.public_lng BETWEEN $11 AND $12 AND %s <= $13`,
			haversineExpr,
		)
		args = append(args, lat, lng, lat-latDelta, lat+latDelta, lng-lngDelta, lng+lngDelta, f.RadiusKM)
		orderBy = fmt.Sprintf(` ORDER BY %s ASC, lower(s.title), s.id`, haversineExpr)
	}
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback()

	var total int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM service_listings s JOIN partners p ON p.id=s.partner_id WHERE `+predicate, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	limitPos := len(args) + 1
	offsetPos := len(args) + 2
	rows, err := tx.QueryContext(ctx, fmt.Sprintf(`SELECT %s FROM service_listings s JOIN partners p ON p.id=s.partner_id WHERE %s%s LIMIT $%d OFFSET $%d`, serviceColumns, predicate, orderBy, limitPos, offsetPos), append(args, f.Limit, f.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	items := []ServiceListing{}
	for rows.Next() {
		v, err := scanService(rows)
		if err != nil {
			rows.Close()
			return nil, 0, err
		}
		if hasRadius && v.PublicLat != nil && v.PublicLng != nil {
			d := haversineKM(*f.Lat, *f.Lng, *v.PublicLat, *v.PublicLng)
			v.DistanceKM = &d
		}
		items = append(items, *v)
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
func (s *Store) PublicService(ctx context.Context, slug string) (*ServiceListing, error) {
	return s.getService(ctx,
		`SELECT `+serviceColumns+` FROM service_listings s JOIN partners p ON p.id=s.partner_id WHERE s.slug=$1 AND s.state='published' AND p.status='active'`,
		slug,
	)
}

func (s *Store) getService(ctx context.Context, query string, args ...any) (*ServiceListing, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	v, err := scanService(tx.QueryRowContext(ctx, query, args...))
	if err != nil {
		return nil, err
	}
	v.WeeklyHours, err = loadServiceWeeklyHours(ctx, tx, v.ID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, wrapDBError(err)
	}
	return v, nil
}

// OwnServices scopes the query in SQL; no caller-supplied partner ID is trusted.
func (s *Store) OwnServices(ctx context.Context, userID, limit, offset int) ([]ServiceListing, int, error) {
	return s.OwnServicesByState(ctx, userID, "", limit, offset)
}

// OwnServicesByState applies publication-state filtering before counting and paging.
func (s *Store) OwnServicesByState(ctx context.Context, userID int, state string, limit, offset int) ([]ServiceListing, int, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	if (state != "" && state != "draft" && state != "published" && state != "archived") || userID <= 0 || limit < 1 || limit > 50 || offset < 0 || offset > 50000 {
		return nil, 0, ErrServiceNotFound
	}
	const scope = `JOIN partner_users pu ON pu.partner_id=s.partner_id JOIN users u ON u.id=pu.user_id WHERE pu.user_id=$1 AND u.role='partner' AND u.deleted_at IS NULL AND p.status='active' AND ($2='' OR s.state=$2)`
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback()

	var total int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM service_listings s JOIN partners p ON p.id=s.partner_id `+scope, userID, state).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT `+serviceColumns+` FROM service_listings s JOIN partners p ON p.id=s.partner_id `+scope+` ORDER BY s.updated_at DESC,s.id DESC LIMIT $3 OFFSET $4`, userID, state, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	items := []ServiceListing{}
	for rows.Next() {
		v, err := scanService(rows)
		if err != nil {
			rows.Close()
			return nil, 0, err
		}
		items = append(items, *v)
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

// OwnService reads a draft or archived listing only through current shop membership.
func (s *Store) OwnService(ctx context.Context, userID, id int) (*ServiceListing, error) {
	return s.getService(ctx,
		`SELECT `+serviceColumns+` FROM service_listings s JOIN partners p ON p.id=s.partner_id JOIN partner_users pu ON pu.partner_id=s.partner_id JOIN users u ON u.id=pu.user_id WHERE s.id=$1 AND u.id=$2 AND u.role='partner' AND u.deleted_at IS NULL AND p.status='active'`,
		id, userID,
	)
}

// SaveService locks membership and the shop before the listing. Team removal,
// suspension and role changes cannot race an already-authorized write.
// The audit insert commits atomically with the content change.
func (s *Store) SaveService(ctx context.Context, userID int, v ServiceListing) (*ServiceListing, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	if v.Timezone == "" {
		v.Timezone = "Africa/Nairobi"
	}
	var saved *ServiceListing
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		saved = nil
		id := v.ID
		var partnerID int
		err := tx.QueryRowContext(ctx, `SELECT p.id FROM partners p JOIN partner_users pu ON pu.partner_id=p.id JOIN users u ON u.id=pu.user_id WHERE u.id=$1 AND u.role='partner' AND u.deleted_at IS NULL AND p.status='active' FOR SHARE OF p,pu,u`, userID).Scan(&partnerID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrServiceNotFound
		}
		if err != nil {
			return err
		}
		var saveLat, saveLng sql.NullFloat64
		if v.DeliveryMode == "premises" && v.PublicLat != nil && v.PublicLng != nil {
			saveLat = sql.NullFloat64{Float64: *v.PublicLat, Valid: true}
			saveLng = sql.NullFloat64{Float64: *v.PublicLng, Valid: true}
		}
		action := "service.create"
		if id > 0 {
			var version int
			if err := tx.QueryRowContext(ctx, `SELECT version FROM service_listings WHERE id=$1 AND partner_id=$2 FOR UPDATE`, id, partnerID).Scan(&version); errors.Is(err, sql.ErrNoRows) {
				return ErrServiceNotFound
			} else if err != nil {
				return err
			}
			if version != v.Version {
				return ErrServiceConflict
			}
			action = "service.update"
			_, err = tx.ExecContext(ctx, `UPDATE service_listings SET title=$1,description=$2,category=$3,duration_minutes=$4,price_mode=$5,price_cents=$6,currency=$7,public_area=$8,delivery_mode=$9,state=$10,timezone=$11,public_lat=$12,public_lng=$13,version=version+1 WHERE id=$14 AND partner_id=$15`, v.Title, v.Description, v.Category, v.DurationMinutes, v.PriceMode, v.PriceCents, v.Currency, v.PublicArea, v.DeliveryMode, v.State, v.Timezone, saveLat, saveLng, id, partnerID)
		} else {
			err = tx.QueryRowContext(ctx, `INSERT INTO service_listings(partner_id,slug,title,description,category,duration_minutes,price_mode,price_cents,currency,public_area,delivery_mode,state,timezone,public_lat,public_lng) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15) RETURNING id`, partnerID, v.Slug, v.Title, v.Description, v.Category, v.DurationMinutes, v.PriceMode, v.PriceCents, v.Currency, v.PublicArea, v.DeliveryMode, v.State, v.Timezone, saveLat, saveLng).Scan(&id)
		}
		if err != nil {
			return wrapDBError(err)
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM service_weekly_hours WHERE service_listing_id=$1`, id); err != nil {
			return err
		}
		for _, hour := range v.WeeklyHours {
			if _, err = tx.ExecContext(ctx, `INSERT INTO service_weekly_hours(service_listing_id,weekday,opens_at,closes_at) VALUES($1,$2,$3::time,$4::time)`, id, hour.Weekday, hour.OpensAt, hour.ClosesAt); err != nil {
				return wrapDBError(err)
			}
		}
		saved, err = scanService(tx.QueryRowContext(ctx, `SELECT `+serviceColumns+` FROM service_listings s JOIN partners p ON p.id=s.partner_id WHERE s.id=$1`, id))
		if err != nil {
			return err
		}
		saved.WeeklyHours, err = loadServiceWeeklyHours(ctx, tx, id)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO audit_logs(actor_user_id,action,target_type,target_id,details)VALUES($1,$2,'service',$3,$4)`, userID, action, id, fmt.Sprintf("state=%s,version=%d", saved.State, saved.Version))
		return err
	})
	return saved, err
}

func haversineKM(lat1, lng1, lat2, lng2 float64) float64 {
	const earthRadiusKM = 6371.0
	dLat := (lat2 - lat1) * math.Pi / 180.0
	dLng := (lng2 - lng1) * math.Pi / 180.0
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180.0)*math.Cos(lat2*math.Pi/180.0)*
			math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * earthRadiusKM * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

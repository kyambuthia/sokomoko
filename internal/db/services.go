package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/kyambuthia/sokomoko/internal/money"
)

var (
	ErrServiceNotFound = errors.New("service listing not found")
	ErrServiceConflict = errors.New("service listing changed; reload before editing")
)

type ServiceListing struct {
	ID              int         `json:"id"`
	PartnerID       int         `json:"partner_id"`
	PartnerName     string      `json:"partner_name"`
	Slug            string      `json:"slug"`
	Title           string      `json:"title"`
	Description     string      `json:"description"`
	Category        string      `json:"category"`
	DurationMinutes int         `json:"duration_minutes"`
	PriceMode       string      `json:"price_mode"`
	PriceCents      money.Cents `json:"price_cents"`
	Currency        string      `json:"currency"`
	PublicArea      string      `json:"public_area"`
	DeliveryMode    string      `json:"delivery_mode"`
	State           string      `json:"state"`
	Version         int         `json:"version"`
	CreatedAt       time.Time   `json:"created_at"`
	UpdatedAt       time.Time   `json:"updated_at"`
}

type ServiceFilter struct {
	Query, Category, Area, DeliveryMode string
	PartnerID, Limit, Offset            int
}

const serviceColumns = `s.id,s.partner_id,p.name,s.slug,s.title,s.description,s.category,s.duration_minutes,s.price_mode,s.price_cents,s.currency,s.public_area,s.delivery_mode,s.state,s.version,s.created_at,s.updated_at`

func scanService(row interface{ Scan(...any) error }) (*ServiceListing, error) {
	v := &ServiceListing{}
	err := row.Scan(&v.ID, &v.PartnerID, &v.PartnerName, &v.Slug, &v.Title, &v.Description, &v.Category, &v.DurationMinutes, &v.PriceMode, &v.PriceCents, &v.Currency, &v.PublicArea, &v.DeliveryMode, &v.State, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrServiceNotFound
	}
	return v, err
}

// BrowseServices exposes only intentionally published offerings of active shops.
// STRPOS treats %, _, and user punctuation literally rather than as patterns.
func (s *Store) BrowseServices(ctx context.Context, f ServiceFilter) ([]ServiceListing, int, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	if f.PartnerID < 0 || f.PartnerID > 2147483647 || f.Limit < 1 || f.Limit > 50 || f.Offset < 0 || f.Offset > 50000 {
		return nil, 0, ErrServiceNotFound
	}
	const predicate = `s.state='published' AND p.status='active' AND ($1='' OR strpos(lower(s.title||' '||s.description||' '||p.name),lower($1))>0) AND ($2='' OR s.category=$2) AND ($3='' OR strpos(lower(s.public_area),lower($3))>0) AND ($4='' OR s.delivery_mode=$4) AND ($5=0 OR s.partner_id=$5)`
	args := []any{f.Query, f.Category, f.Area, f.DeliveryMode, f.PartnerID}
	var total int
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM service_listings s JOIN partners p ON p.id=s.partner_id WHERE `+predicate, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+serviceColumns+` FROM service_listings s JOIN partners p ON p.id=s.partner_id WHERE `+predicate+` ORDER BY lower(s.title),s.id LIMIT $6 OFFSET $7`, append(args, f.Limit, f.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []ServiceListing{}
	for rows.Next() {
		v, err := scanService(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, *v)
	}
	return items, total, rows.Err()
}
func (s *Store) PublicService(ctx context.Context, slug string) (*ServiceListing, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	return scanService(s.DB.QueryRowContext(ctx, `SELECT `+serviceColumns+` FROM service_listings s JOIN partners p ON p.id=s.partner_id WHERE s.slug=$1 AND s.state='published' AND p.status='active'`, slug))
}

// OwnServices scopes the query in SQL; no caller-supplied partner ID is trusted.
func (s *Store) OwnServices(ctx context.Context, userID, limit, offset int) ([]ServiceListing, int, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	if userID <= 0 || limit < 1 || limit > 50 || offset < 0 || offset > 50000 {
		return nil, 0, ErrServiceNotFound
	}
	const scope = `JOIN partner_users pu ON pu.partner_id=s.partner_id JOIN users u ON u.id=pu.user_id WHERE pu.user_id=$1 AND u.role='partner' AND u.deleted_at IS NULL AND p.status='active'`
	var total int
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM service_listings s JOIN partners p ON p.id=s.partner_id `+scope, userID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+serviceColumns+` FROM service_listings s JOIN partners p ON p.id=s.partner_id `+scope+` ORDER BY s.updated_at DESC,s.id DESC LIMIT $2 OFFSET $3`, userID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []ServiceListing{}
	for rows.Next() {
		v, err := scanService(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, *v)
	}
	return items, total, rows.Err()
}

// OwnService reads a draft or archived listing only through current shop membership.
func (s *Store) OwnService(ctx context.Context, userID, id int) (*ServiceListing, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	return scanService(s.DB.QueryRowContext(ctx, `SELECT `+serviceColumns+` FROM service_listings s JOIN partners p ON p.id=s.partner_id JOIN partner_users pu ON pu.partner_id=s.partner_id JOIN users u ON u.id=pu.user_id WHERE s.id=$1 AND u.id=$2 AND u.role='partner' AND u.deleted_at IS NULL AND p.status='active'`, id, userID))
}

// SaveService locks membership and the shop before the listing. Team removal,
// suspension and role changes cannot race an already-authorized write.
// The audit insert commits atomically with the content change.
func (s *Store) SaveService(ctx context.Context, userID int, v ServiceListing) (*ServiceListing, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
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
			_, err = tx.ExecContext(ctx, `UPDATE service_listings SET title=$1,description=$2,category=$3,duration_minutes=$4,price_mode=$5,price_cents=$6,currency=$7,public_area=$8,delivery_mode=$9,state=$10,version=version+1 WHERE id=$11 AND partner_id=$12`, v.Title, v.Description, v.Category, v.DurationMinutes, v.PriceMode, v.PriceCents, v.Currency, v.PublicArea, v.DeliveryMode, v.State, id, partnerID)
		} else {
			err = tx.QueryRowContext(ctx, `INSERT INTO service_listings(partner_id,slug,title,description,category,duration_minutes,price_mode,price_cents,currency,public_area,delivery_mode,state) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING id`, partnerID, v.Slug, v.Title, v.Description, v.Category, v.DurationMinutes, v.PriceMode, v.PriceCents, v.Currency, v.PublicArea, v.DeliveryMode, v.State).Scan(&id)
		}
		if err != nil {
			return wrapDBError(err)
		}
		saved, err = scanService(tx.QueryRowContext(ctx, `SELECT `+serviceColumns+` FROM service_listings s JOIN partners p ON p.id=s.partner_id WHERE s.id=$1`, id))
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO audit_logs(actor_user_id,action,target_type,target_id,details)VALUES($1,$2,'service',$3,$4)`, userID, action, id, fmt.Sprintf("state=%s,version=%d", saved.State, saved.Version))
		return err
	})
	return saved, err
}

package db

import (
	"database/sql"
	"errors"
	"strings"
)

func (s *Store) GetStoreSettings() (*StoreSettings, error) {
	ctx, cancel := s.ctx()
	defer cancel()

	settings := &StoreSettings{}
	err := s.DB.QueryRowContext(ctx,
		`SELECT store_name, store_slug, description, contact_email, initialized_at, updated_at
		 FROM store_settings
		 WHERE id = 1`,
	).Scan(
		&settings.StoreName,
		&settings.StoreSlug,
		&settings.Description,
		&settings.ContactEmail,
		&settings.InitializedAt,
		&settings.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return settings, nil
}

func (s *Store) UpsertStoreSettings(settings StoreSettings) error {
	ctx, cancel := s.ctx()
	defer cancel()
	_, err := s.DB.ExecContext(ctx,
		`INSERT INTO store_settings (id, store_name, store_slug, description, contact_email)
		 VALUES (1, $1, $2, $3, $4)
		 ON CONFLICT (id) DO UPDATE SET
		   store_name = EXCLUDED.store_name,
		   store_slug = EXCLUDED.store_slug,
		   description = EXCLUDED.description,
		   contact_email = EXCLUDED.contact_email`,
		strings.TrimSpace(settings.StoreName),
		strings.TrimSpace(settings.StoreSlug),
		strings.TrimSpace(settings.Description),
		strings.TrimSpace(settings.ContactEmail),
	)
	return wrapDBError(err)
}

// UpsertPartner creates or updates a partner identified by slug and returns its id.
func (s *Store) UpsertPartner(partner Partner) (int64, error) {
	ctx, cancel := s.ctx()
	defer cancel()

	status := strings.TrimSpace(partner.Status)
	if status == "" {
		status = "active"
	}
	var id int64
	err := s.DB.QueryRowContext(ctx,
		`INSERT INTO partners (name, slug, contact_email, status)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (slug) DO UPDATE SET
		   name = EXCLUDED.name,
		   contact_email = EXCLUDED.contact_email,
		   status = EXCLUDED.status
		 RETURNING id`,
		strings.TrimSpace(partner.Name),
		strings.ToLower(strings.TrimSpace(partner.Slug)),
		strings.ToLower(strings.TrimSpace(partner.ContactEmail)),
		status,
	).Scan(&id)
	return id, wrapDBError(err)
}

func (s *Store) GetPartnerBySlug(slug string) (*Partner, error) {
	ctx, cancel := s.ctx()
	defer cancel()

	partner := &Partner{}
	err := s.DB.QueryRowContext(ctx,
		`SELECT id, name, slug, contact_email, status, created_at, updated_at
		 FROM partners WHERE slug = $1`,
		strings.ToLower(strings.TrimSpace(slug)),
	).Scan(&partner.ID, &partner.Name, &partner.Slug, &partner.ContactEmail, &partner.Status, &partner.CreatedAt, &partner.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return partner, nil
}

func (s *Store) ListPartners() ([]Partner, error) {
	ctx, cancel := s.ctx()
	defer cancel()

	rows, err := s.DB.QueryContext(ctx,
		`SELECT id, name, slug, contact_email, status, created_at, updated_at
		 FROM partners ORDER BY lower(name)`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	partners := []Partner{}
	for rows.Next() {
		var partner Partner
		if err := rows.Scan(&partner.ID, &partner.Name, &partner.Slug, &partner.ContactEmail, &partner.Status, &partner.CreatedAt, &partner.UpdatedAt); err != nil {
			return nil, err
		}
		partners = append(partners, partner)
	}
	return partners, rows.Err()
}

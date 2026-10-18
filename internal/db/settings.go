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

// PartnerIDForUser resolves an explicitly assigned active partner membership.
func (s *Store) PartnerIDForUser(userID int) (int, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	var id int
	err := s.DB.QueryRowContext(ctx, `SELECT p.id FROM partners p
 JOIN partner_users pu ON pu.partner_id = p.id
 JOIN users u ON u.id = pu.user_id
 WHERE pu.user_id = $1 AND p.status = 'active'
 AND u.role = 'partner' AND u.deleted_at IS NULL`, userID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}

// CreatePartner inserts a new active partner. A taken slug returns
// ErrUniqueConstraint.
func (s *Store) CreatePartner(partner Partner) (int64, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	var id int64
	err := s.DB.QueryRowContext(ctx,
		`INSERT INTO partners (name, slug, contact_email) VALUES ($1, $2, $3) RETURNING id`,
		strings.TrimSpace(partner.Name),
		strings.ToLower(strings.TrimSpace(partner.Slug)),
		strings.ToLower(strings.TrimSpace(partner.ContactEmail)),
	).Scan(&id)
	return id, wrapDBError(err)
}

func (s *Store) GetPartnerByID(id int) (*Partner, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	partner := &Partner{}
	err := s.DB.QueryRowContext(ctx,
		`SELECT id, name, slug, contact_email, status, created_at, updated_at FROM partners WHERE id = $1`, id,
	).Scan(&partner.ID, &partner.Name, &partner.Slug, &partner.ContactEmail, &partner.Status, &partner.CreatedAt, &partner.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return partner, nil
}

// SetPartnerStatus marks a partner active or suspended. Suspending hides the
// partner's products from customers and stops its accounts from seeing
// partner data. It reports false when the partner does not exist.
// SetPartnerStatus changes a partner's status. Suspending also signs out the
// partner's own logins in the same transaction.
func (s *Store) SetPartnerStatus(id int, status string) (bool, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	found := false
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, "UPDATE partners SET status = $1 WHERE id = $2", status, id)
		if err != nil {
			return wrapDBError(err)
		}
		n, err := rowsAffected(result)
		if err != nil {
			return err
		}
		found = n > 0
		if !found || status != "suspended" {
			return nil
		}
		_, err = tx.ExecContext(ctx,
			"DELETE FROM sessions WHERE user_id IN (SELECT user_id FROM partner_users WHERE partner_id = $1)", id)
		return err
	})
	return found, err
}

// CreatePartnerUser creates a login with the partner role and links it to
// partnerID in one transaction.
func (s *Store) CreatePartnerUser(user User, partnerID int) (int64, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	var id int64
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx,
			`INSERT INTO users (username, email, password_hash, role) VALUES ($1, $2, $3, 'partner') RETURNING id`,
			strings.TrimSpace(user.Username), strings.ToLower(strings.TrimSpace(user.Email)), user.PasswordHash,
		).Scan(&id); err != nil {
			return wrapDBError(err)
		}
		_, err := tx.ExecContext(ctx, "INSERT INTO partner_users (user_id, partner_id) VALUES ($1, $2)", id, partnerID)
		return wrapDBError(err)
	})
	return id, err
}

// PartnerAccount is a partner login and the partner it belongs to.
type PartnerAccount struct {
	UserID      int
	Username    string
	Email       string
	PartnerID   int
	PartnerName string
}

// ListPartnerAccounts returns active partner logins ordered by partner name.
func (s *Store) ListPartnerAccounts() ([]PartnerAccount, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	rows, err := s.DB.QueryContext(ctx,
		`SELECT u.id, u.username, u.email, p.id, p.name
		 FROM partner_users pu
		 JOIN users u ON u.id = pu.user_id AND u.deleted_at IS NULL
		 JOIN partners p ON p.id = pu.partner_id
		 ORDER BY lower(p.name), lower(u.username)`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	accounts := []PartnerAccount{}
	for rows.Next() {
		var a PartnerAccount
		if err := rows.Scan(&a.UserID, &a.Username, &a.Email, &a.PartnerID, &a.PartnerName); err != nil {
			return nil, err
		}
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}

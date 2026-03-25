package db

import (
	"fmt"
	"strings"
)

func (s *Store) GetStoreSettings() (*StoreSettings, error) {
	row := s.DB.QueryRow(
		`SELECT id, store_name, store_slug, description, contact_email, initialized_at, updated_at
		 FROM store_settings
		 WHERE id = 1`,
	)
	settings := &StoreSettings{}
	err := row.Scan(
		&settings.ID,
		&settings.StoreName,
		&settings.StoreSlug,
		&settings.Description,
		&settings.ContactEmail,
		&settings.InitializedAt,
		&settings.UpdatedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			return nil, nil
		}
		return nil, err
	}
	return settings, nil
}

func (s *Store) UpsertStoreSettings(settings StoreSettings) error {
	stmt, err := s.DB.Prepare(
		`INSERT INTO store_settings (id, store_name, store_slug, description, contact_email)
		 VALUES (1, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   store_name = excluded.store_name,
		   store_slug = excluded.store_slug,
		   description = excluded.description,
		   contact_email = excluded.contact_email,
		   updated_at = CURRENT_TIMESTAMP`,
	)
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(settings.StoreName, settings.StoreSlug, settings.Description, settings.ContactEmail)
	return err
}

func (s *Store) CountProducts() (int, error) {
	row := s.DB.QueryRow("SELECT COUNT(*) FROM products WHERE deleted_at IS NULL")
	var count int
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) CountActiveSessions() (int, error) {
	row := s.DB.QueryRow("SELECT COUNT(*) FROM sessions WHERE expires_at > CURRENT_TIMESTAMP")
	var count int
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) ListUsersByRoles(roles []string) ([]User, error) {
	if len(roles) == 0 {
		return []User{}, nil
	}

	placeholders := make([]string, len(roles))
	args := make([]interface{}, 0, len(roles))
	for i, role := range roles {
		placeholders[i] = "?"
		args = append(args, role)
	}

	query := fmt.Sprintf(
		`SELECT id, username, email, password_hash, salt, role, slug, created_at, updated_at, deleted_at
		 FROM users
		 WHERE role IN (%s) AND deleted_at IS NULL
		 ORDER BY role, username`,
		strings.Join(placeholders, ","),
	)

	rows, err := s.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := []User{}
	for rows.Next() {
		user := User{}
		err := rows.Scan(
			&user.ID,
			&user.Username,
			&user.Email,
			&user.PasswordHash,
			&user.Salt,
			&user.Role,
			&user.Slug,
			&user.CreatedAt,
			&user.UpdatedAt,
			&user.DeletedAt,
		)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, nil
}

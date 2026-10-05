package db

import (
	"database/sql"
	"errors"
	"strings"
)

const userColumns = "id, username, email, password_hash, role, created_at, updated_at, deleted_at"

func scanUser(row interface{ Scan(...any) error }) (*User, error) {
	user := &User{}
	err := row.Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.PasswordHash,
		&user.Role,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.DeletedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return user, nil
}

// CreateUser inserts a new user. Emails are stored lower-cased; usernames keep
// their case but are unique case-insensitively.
func (s *Store) CreateUser(user User) (int64, error) {
	ctx, cancel := s.ctx()
	defer cancel()

	role := strings.TrimSpace(user.Role)
	if role == "" {
		role = RoleUser
	}

	var id int64
	err := s.DB.QueryRowContext(ctx,
		`INSERT INTO users (username, email, password_hash, role)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id`,
		strings.TrimSpace(user.Username),
		strings.ToLower(strings.TrimSpace(user.Email)),
		user.PasswordHash,
		role,
	).Scan(&id)
	if err != nil {
		return 0, wrapDBError(err)
	}
	return id, nil
}

func (s *Store) GetUserByUsername(username string) (*User, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	return scanUser(s.DB.QueryRowContext(ctx,
		"SELECT "+userColumns+" FROM users WHERE lower(username) = lower($1) AND deleted_at IS NULL",
		strings.TrimSpace(username),
	))
}

func (s *Store) GetUserByID(id int) (*User, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	return scanUser(s.DB.QueryRowContext(ctx,
		"SELECT "+userColumns+" FROM users WHERE id = $1 AND deleted_at IS NULL",
		id,
	))
}

func (s *Store) GetUserByEmail(email string) (*User, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	return scanUser(s.DB.QueryRowContext(ctx,
		"SELECT "+userColumns+" FROM users WHERE lower(email) = lower($1) AND deleted_at IS NULL",
		strings.TrimSpace(email),
	))
}

func (s *Store) CountUsersByRole(role string) (int, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	var count int
	err := s.DB.QueryRowContext(ctx,
		"SELECT count(*) FROM users WHERE role = $1 AND deleted_at IS NULL", role,
	).Scan(&count)
	return count, err
}

func (s *Store) HasAdminUser() (bool, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	var exists bool
	err := s.DB.QueryRowContext(ctx,
		"SELECT EXISTS (SELECT 1 FROM users WHERE role = $1 AND deleted_at IS NULL)", RoleAdmin,
	).Scan(&exists)
	return exists, err
}

// UpdateUserPassword replaces a password hash and revokes all sessions.
func (s *Store) UpdateUserPassword(userID int, passwordHash string) error {
	ctx, cancel := s.ctx()
	defer cancel()

	return s.withTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx,
			"UPDATE users SET password_hash = $1 WHERE id = $2 AND deleted_at IS NULL",
			passwordHash, userID,
		)
		if err != nil {
			return err
		}
		affected, err := rowsAffected(result)
		if err != nil || affected == 0 {
			return err
		}
		_, err = tx.ExecContext(ctx, "DELETE FROM sessions WHERE user_id = $1", userID)
		return err
	})
}

// UpdateUser updates profile fields and role.
func (s *Store) UpdateUser(user User) error {
	ctx, cancel := s.ctx()
	defer cancel()
	_, err := s.DB.ExecContext(ctx,
		`UPDATE users SET username = $1, email = $2, password_hash = $3, role = $4
		 WHERE id = $5 AND deleted_at IS NULL`,
		strings.TrimSpace(user.Username),
		strings.ToLower(strings.TrimSpace(user.Email)),
		user.PasswordHash,
		user.Role,
		user.ID,
	)
	return wrapDBError(err)
}

// DeleteUser soft-deletes a user and revokes their sessions.
func (s *Store) DeleteUser(id int) error {
	ctx, cancel := s.ctx()
	defer cancel()

	return s.withTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			"UPDATE users SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL", id,
		); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, "DELETE FROM sessions WHERE user_id = $1", id)
		return err
	})
}

func (s *Store) ListUsersByRoles(roles []string) ([]User, error) {
	if len(roles) == 0 {
		return []User{}, nil
	}
	ctx, cancel := s.ctx()
	defer cancel()

	rows, err := s.DB.QueryContext(ctx,
		"SELECT "+userColumns+` FROM users
		 WHERE role = ANY($1) AND deleted_at IS NULL
		 ORDER BY role, lower(username)`,
		roles,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := []User{}
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, *user)
	}
	return users, rows.Err()
}

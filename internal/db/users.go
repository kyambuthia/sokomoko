package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
func (s *Store) CreateUser(ctx context.Context, user User) (int64, error) {
	ctx, cancel := s.queryContext(ctx)
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

func (s *Store) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	return scanUser(s.DB.QueryRowContext(ctx,
		"SELECT "+userColumns+" FROM users WHERE lower(username) = lower($1) AND deleted_at IS NULL",
		strings.TrimSpace(username),
	))
}

func (s *Store) GetUserByID(ctx context.Context, id int) (*User, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	return scanUser(s.DB.QueryRowContext(ctx,
		"SELECT "+userColumns+" FROM users WHERE id = $1 AND deleted_at IS NULL",
		id,
	))
}

func (s *Store) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	return scanUser(s.DB.QueryRowContext(ctx,
		"SELECT "+userColumns+" FROM users WHERE lower(email) = lower($1) AND deleted_at IS NULL",
		strings.TrimSpace(email),
	))
}

func (s *Store) CountUsersByRole(ctx context.Context, role string) (int, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	var count int
	err := s.DB.QueryRowContext(ctx,
		"SELECT count(*) FROM users WHERE role = $1 AND deleted_at IS NULL", role,
	).Scan(&count)
	return count, err
}

// DashboardCounts is the admin header summary, read in one round trip.
type DashboardCounts struct {
	Products, Sessions, Admins, Staff, Users int
}

func (s *Store) DashboardCounts(ctx context.Context) (DashboardCounts, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	var c DashboardCounts
	err := s.DB.QueryRowContext(ctx, `SELECT
		(SELECT count(*) FROM products WHERE deleted_at IS NULL),
		(SELECT count(*) FROM sessions WHERE expires_at > now()),
		count(*) FILTER (WHERE role = 'admin'),
		count(*) FILTER (WHERE role = 'staff'),
		count(*) FILTER (WHERE role = 'user')
		FROM users WHERE deleted_at IS NULL`).
		Scan(&c.Products, &c.Sessions, &c.Admins, &c.Staff, &c.Users)
	return c, err
}

func (s *Store) HasAdminUser(ctx context.Context) (bool, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	var exists bool
	err := s.DB.QueryRowContext(ctx,
		"SELECT EXISTS (SELECT 1 FROM users WHERE role = $1 AND deleted_at IS NULL)", RoleAdmin,
	).Scan(&exists)
	return exists, err
}

// UpdateUserPassword replaces a password hash and revokes all sessions.
func (s *Store) UpdateUserPassword(ctx context.Context, userID int, passwordHash string) error {
	ctx, cancel := s.queryContext(ctx)
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
func (s *Store) UpdateUser(ctx context.Context, user User) error {
	ctx, cancel := s.queryContext(ctx)
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
func (s *Store) DeleteUser(ctx context.Context, id int) error {
	ctx, cancel := s.queryContext(ctx)
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

// ListDeactivatedUsers returns the most recently deactivated non-admin
// accounts, newest first.
func (s *Store) ListDeactivatedUsers(ctx context.Context, limit int) ([]User, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	rows, err := s.DB.QueryContext(ctx,
		"SELECT "+userColumns+` FROM users
		 WHERE deleted_at IS NOT NULL AND role <> 'admin'
		 ORDER BY deleted_at DESC, id DESC LIMIT $1`, limit)
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

// ReactivateUser restores a deactivated non-admin account. Usernames and
// emails stay unique across deactivated rows, so restoring cannot collide.
func (s *Store) ReactivateUser(ctx context.Context, id int) (bool, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	result, err := s.DB.ExecContext(ctx,
		"UPDATE users SET deleted_at = NULL WHERE id = $1 AND deleted_at IS NOT NULL AND role <> 'admin'", id)
	if err != nil {
		return false, err
	}
	n, err := rowsAffected(result)
	return n > 0, err
}

// UserSearch selects active users by role, optionally matching query against
// username or email, one page at a time.
type UserSearch struct {
	Roles  []string
	Query  string
	Limit  int
	Offset int
}

// ListUsersByRoles returns one page of active users ordered by role and
// username, and the total number that match.
func (s *Store) ListUsersByRoles(ctx context.Context, search UserSearch) ([]User, int, error) {
	if len(search.Roles) == 0 {
		return []User{}, 0, nil
	}
	ctx, cancel := s.queryContext(ctx)
	defer cancel()

	where := "role = ANY($1) AND deleted_at IS NULL"
	args := []any{search.Roles}
	if q := strings.TrimSpace(search.Query); q != "" {
		args = append(args, "%"+escapeLike(q)+"%")
		where += " AND (username ILIKE $2 OR email ILIKE $2)"
	}

	var total int
	if err := s.DB.QueryRowContext(ctx, "SELECT count(*) FROM users WHERE "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, search.Limit, search.Offset)
	rows, err := s.DB.QueryContext(ctx,
		"SELECT "+userColumns+" FROM users WHERE "+where+
			fmt.Sprintf(" ORDER BY role, lower(username), id LIMIT $%d OFFSET $%d", len(args)-1, len(args)),
		args...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	users := []User{}
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, 0, err
		}
		users = append(users, *user)
	}
	return users, total, rows.Err()
}

// ErrAdminExists is returned by BootstrapAdmin when an admin already exists.
var ErrAdminExists = errors.New("an admin account already exists")

const bootstrapLockKey int64 = 0x736f6b6f61646d6e // "sokoadmn"

// StaffNamer returns the username and email to try for the i-th staff account
// (1-based) on the given attempt (0-based), used to skip taken usernames.
type StaffNamer func(i, attempt int) (username, email string)

// BootstrapAdmin creates the first admin and staffCount staff accounts in one
// transaction. Concurrent callers serialize on an advisory lock, and the call
// fails with ErrAdminExists if an admin was created first, so first-run setup
// can never leave a partial or duplicate set of privileged accounts.
// staffPasswordHashes must have one entry per staff account.
func (s *Store) BootstrapAdmin(ctx context.Context, admin User, staffPasswordHashes []string, name StaffNamer) ([]User, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()

	var created []User
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		created = nil
		if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", bootstrapLockKey); err != nil {
			return err
		}
		var exists bool
		if err := tx.QueryRowContext(ctx,
			"SELECT EXISTS (SELECT 1 FROM users WHERE role = $1 AND deleted_at IS NULL)", RoleAdmin,
		).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return ErrAdminExists
		}

		if _, err := tx.ExecContext(ctx,
			"INSERT INTO users (username, email, password_hash, role) VALUES ($1, $2, $3, $4)",
			strings.TrimSpace(admin.Username), strings.ToLower(strings.TrimSpace(admin.Email)), admin.PasswordHash, RoleAdmin,
		); err != nil {
			return wrapDBError(err)
		}

		for i, hash := range staffPasswordHashes {
			inserted := false
			for attempt := 0; attempt < 10 && !inserted; attempt++ {
				username, email := name(i+1, attempt)
				var id int
				err := tx.QueryRowContext(ctx,
					`INSERT INTO users (username, email, password_hash, role)
					 VALUES ($1, $2, $3, $4)
					 ON CONFLICT DO NOTHING
					 RETURNING id`,
					username, strings.ToLower(email), hash, RoleStaff,
				).Scan(&id)
				if errors.Is(err, sql.ErrNoRows) {
					continue
				}
				if err != nil {
					return wrapDBError(err)
				}
				created = append(created, User{ID: id, Username: username, Email: strings.ToLower(email), Role: RoleStaff})
				inserted = true
			}
			if !inserted {
				return fmt.Errorf("could not find a free username for staff account %d", i+1)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

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

// ChangeUserPassword compares the hash verified by the request, replaces it,
// revokes old credentials, and creates the replacement session atomically.
func (s *Store) ChangeUserPassword(ctx context.Context, userID int, expectedPasswordHash, passwordHash string, session Session) error {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()

	return s.withTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx,
			"UPDATE users SET password_hash = $1 WHERE id = $2 AND deleted_at IS NULL AND password_hash = $3",
			passwordHash, userID, expectedPasswordHash,
		)
		if err != nil {
			return err
		}
		affected, err := rowsAffected(result)
		if err != nil {
			return err
		}
		if affected == 0 {
			return ErrPasswordChanged
		}
		if err := revokeUserCredentialsTx(ctx, tx, userID); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx,
			"INSERT INTO sessions (id, user_id, csrf_token, expires_at) VALUES ($1, $2, $3, $4)",
			hashOpaqueToken(session.ID), userID, session.CSRFToken, session.ExpiresAt.UTC(),
		)
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
	return s.deleteUser(ctx, 0, id, false)
}

// DeleteUserForActor revalidates an admin actor in the same transaction as
// user deactivation.
func (s *Store) DeleteUserForActor(ctx context.Context, actorID, targetID int) error {
	return s.deleteUser(ctx, actorID, targetID, true)
}

func (s *Store) deleteUser(ctx context.Context, actorID, id int, requireAdmin bool) error {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()

	return s.withTx(ctx, func(tx *sql.Tx) error {
		// Acquire this before user-row locks so concurrent deactivations cannot
		// deadlock while locking one another as actor and target.
		if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", bootstrapLockKey); err != nil {
			return err
		}
		if requireAdmin {
			if _, _, err := lockWorkspaceActorTx(ctx, tx, actorID, RoleAdmin); err != nil {
				return err
			}
		}
		var role string
		err := tx.QueryRowContext(ctx,
			"SELECT role FROM users WHERE id = $1 AND deleted_at IS NULL FOR UPDATE", id,
		).Scan(&role)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if role == RoleAdmin {
			var activeAdmins int
			if err := tx.QueryRowContext(ctx,
				"SELECT count(*) FROM users WHERE role = $1 AND deleted_at IS NULL",
				RoleAdmin,
			).Scan(&activeAdmins); err != nil {
				return err
			}
			if activeAdmins <= 1 {
				return ErrLastAdmin
			}
		}
		if role == RoleCourier {
			var active bool
			if err := tx.QueryRowContext(ctx,
				`SELECT EXISTS (
				 SELECT 1 FROM courier_deliveries cd JOIN orders o ON o.id=cd.order_id
				 WHERE cd.courier_user_id = $1 AND cd.status IN ('assigned', 'picked_up', 'out_for_delivery')
				 AND o.status NOT IN ('cancelled','delivered') AND o.delivery_status <> 'delivered'
				)`, id,
			).Scan(&active); err != nil {
				return err
			}
			if active {
				return ErrCourierHasActiveDeliveries
			}
		}
		if _, err := tx.ExecContext(ctx,
			"UPDATE users SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL", id,
		); err != nil {
			return err
		}
		return revokeUserCredentialsTx(ctx, tx, id)
	})
}

// ListDeactivatedUsers returns the most recently deactivated accounts, newest
// first.
func (s *Store) ListDeactivatedUsers(ctx context.Context, limit int) ([]User, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	rows, err := s.DB.QueryContext(ctx,
		"SELECT "+userColumns+` FROM users
		 WHERE deleted_at IS NOT NULL
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

// ReactivateUser restores a deactivated account for a trusted internal caller.
func (s *Store) ReactivateUser(ctx context.Context, id int) (bool, error) {
	return s.reactivateUser(ctx, 0, id, false)
}

// ReactivateUserForActor revalidates an admin in the same transaction as
// account reactivation.
func (s *Store) ReactivateUserForActor(ctx context.Context, actorID, targetID int) (bool, error) {
	return s.reactivateUser(ctx, actorID, targetID, true)
}

func (s *Store) reactivateUser(ctx context.Context, actorID, targetID int, requireAdmin bool) (bool, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	found := false
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		if requireAdmin {
			if _, _, err := lockWorkspaceActorTx(ctx, tx, actorID, RoleAdmin); err != nil {
				return err
			}
		}
		result, err := tx.ExecContext(ctx,
			"UPDATE users SET deleted_at = NULL WHERE id = $1 AND deleted_at IS NOT NULL", targetID)
		if err != nil {
			return err
		}
		n, err := rowsAffected(result)
		found = n > 0
		return err
	})
	return found, err
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
	if search.Limit < 1 {
		search.Limit = 50
	} else if search.Limit > 100 {
		search.Limit = 100
	}
	if search.Offset < 0 {
		search.Offset = 0
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
	if total == 0 {
		return []User{}, 0, nil
	}
	lastOffset := ((total - 1) / search.Limit) * search.Limit
	if search.Offset > lastOffset {
		return []User{}, total, nil
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

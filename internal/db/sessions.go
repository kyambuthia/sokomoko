package db

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

func (s *Store) CreateSession(ctx context.Context, sess Session) error {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	return s.withTx(ctx, func(tx *sql.Tx) error {
		var userID int
		err := tx.QueryRowContext(ctx,
			"SELECT id FROM users WHERE id = $1 AND deleted_at IS NULL FOR SHARE",
			sess.UserID,
		).Scan(&userID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrUserInactive
		}
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx,
			"INSERT INTO sessions (id, user_id, csrf_token, expires_at) VALUES ($1, $2, $3, $4)",
			hashOpaqueToken(sess.ID), sess.UserID, sess.CSRFToken, sess.ExpiresAt.UTC(),
		)
		return err
	})
}

// GetSession returns the unexpired session for a raw cookie token.
func (s *Store) GetSession(ctx context.Context, id string) (*Session, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()

	sess := &Session{}
	err := s.DB.QueryRowContext(ctx,
		`SELECT id, user_id, csrf_token, expires_at, created_at
		 FROM sessions
		 WHERE id = $1 AND expires_at > now()`,
		hashOpaqueToken(id),
	).Scan(&sess.ID, &sess.UserID, &sess.CSRFToken, &sess.ExpiresAt, &sess.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return sess, nil
}

// GetActiveUserForSession returns the active user and expiry for a raw
// session cookie token in one query.
func (s *Store) GetActiveUserForSession(ctx context.Context, id string) (*User, time.Time, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()

	user := &User{}
	var expiresAt time.Time
	err := s.DB.QueryRowContext(ctx,
		`SELECT u.id, u.username, u.email, u.password_hash, u.role,
		        u.created_at, u.updated_at, u.deleted_at, ss.expires_at
		 FROM sessions ss
		 JOIN users u ON u.id = ss.user_id
		 WHERE ss.id = $1 AND ss.expires_at > now() AND u.deleted_at IS NULL`,
		hashOpaqueToken(id),
	).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.PasswordHash,
		&user.Role,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.DeletedAt,
		&expiresAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, time.Time{}, nil
	}
	if err != nil {
		return nil, time.Time{}, err
	}
	return user, expiresAt, nil
}

func (s *Store) DeleteSession(ctx context.Context, id string) error {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	_, err := s.DB.ExecContext(ctx, "DELETE FROM sessions WHERE id = $1", hashOpaqueToken(id))
	return err
}

func (s *Store) CleanupSessions(ctx context.Context) error {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	_, err := s.DB.ExecContext(ctx, "DELETE FROM sessions WHERE expires_at <= now()")
	return err
}

func (s *Store) CountActiveSessions(ctx context.Context) (int, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	var count int
	err := s.DB.QueryRowContext(ctx, "SELECT count(*) FROM sessions WHERE expires_at > now()").Scan(&count)
	return count, err
}

func (s *Store) CleanupPasswordResetTokens(ctx context.Context) error {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	if _, err := s.DB.ExecContext(ctx,
		"DELETE FROM password_reset_email_outbox WHERE expires_at <= now()",
	); err != nil {
		return err
	}
	_, err := s.DB.ExecContext(ctx,
		"DELETE FROM password_reset_tokens WHERE used_at IS NOT NULL OR expires_at <= now()",
	)
	return err
}

// CreatePasswordResetToken stores a new reset token, invalidating any earlier
// tokens for the same user.
func (s *Store) CreatePasswordResetToken(ctx context.Context, userID int, token string, expiresAt time.Time) error {
	_, err := s.replacePasswordResetToken(ctx, userID, token, expiresAt, false, nil)
	return err
}

// RequestPasswordResetToken issues at most one reset email per user every five
// minutes, even when requests arrive through different identifiers or replicas.
func (s *Store) RequestPasswordResetToken(ctx context.Context, userID int, token string, expiresAt time.Time) (bool, error) {
	return s.replacePasswordResetToken(ctx, userID, token, expiresAt, true, nil)
}

// RequestPasswordResetTokenWithOutbox atomically creates a reset token and a
// sealed email payload, subject to the same per-user cooldown as the legacy API.
func (s *Store) RequestPasswordResetTokenWithOutbox(ctx context.Context, userID int, token string, expiresAt time.Time, sealedPayload []byte) (bool, error) {
	if len(sealedPayload) == 0 {
		return false, errors.New("sealed password reset email payload is required")
	}
	return s.replacePasswordResetToken(ctx, userID, token, expiresAt, true, sealedPayload)
}

func (s *Store) replacePasswordResetToken(ctx context.Context, userID int, token string, expiresAt time.Time, enforceCooldown bool, sealedPayload []byte) (bool, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()

	created := false
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		created = false
		// Serialize token replacement and cooldown checks with token use.
		var lockedUserID int
		if err := tx.QueryRowContext(ctx, "SELECT id FROM users WHERE id = $1 FOR UPDATE", userID).
			Scan(&lockedUserID); err != nil {
			return err
		}
		if enforceCooldown {
			var blocked bool
			if err := tx.QueryRowContext(ctx,
				`SELECT COALESCE(password_reset_requested_at > clock_timestamp() - interval '5 minutes', FALSE)
				 FROM users WHERE id = $1`, userID,
			).Scan(&blocked); err != nil {
				return err
			}
			if blocked {
				return nil
			}
		}
		// Replacing a token also cancels any older email that has not been sent.
		if _, err := tx.ExecContext(ctx, "DELETE FROM password_reset_email_outbox WHERE user_id = $1", userID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM password_reset_tokens WHERE user_id = $1", userID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO password_reset_tokens (token_hash, user_id, expires_at) VALUES ($1, $2, $3)",
			hashOpaqueToken(token), userID, expiresAt.UTC(),
		); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			"UPDATE users SET password_reset_requested_at = clock_timestamp() WHERE id = $1", userID,
		); err != nil {
			return err
		}
		if len(sealedPayload) > 0 {
			if _, err := tx.ExecContext(ctx,
				"INSERT INTO password_reset_email_outbox (user_id, sealed_payload, expires_at) VALUES ($1, $2, $3)",
				userID, sealedPayload, expiresAt.UTC(),
			); err != nil {
				return err
			}
		}
		created = true
		return nil
	})
	return created, err
}

func (s *Store) GetValidPasswordResetToken(ctx context.Context, token string) (*PasswordResetToken, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()

	resetToken := &PasswordResetToken{}
	err := s.DB.QueryRowContext(ctx,
		`SELECT id, token_hash, user_id, expires_at, used_at, created_at
		 FROM password_reset_tokens
		 WHERE token_hash = $1 AND used_at IS NULL AND expires_at > now()`,
		hashOpaqueToken(token),
	).Scan(
		&resetToken.ID,
		&resetToken.TokenHash,
		&resetToken.UserID,
		&resetToken.ExpiresAt,
		&resetToken.UsedAt,
		&resetToken.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return resetToken, nil
}

// UsePasswordResetToken atomically consumes a valid token, sets the new password
// hash, and revokes all of the user's sessions. It reports false when the token
// is unknown, used, or expired.
func (s *Store) UsePasswordResetToken(ctx context.Context, token, passwordHash string) (bool, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()

	used := false
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		used = false
		var userID int
		err := tx.QueryRowContext(ctx,
			`SELECT user_id FROM password_reset_tokens
			 WHERE token_hash = $1 AND used_at IS NULL AND expires_at > now()`,
			hashOpaqueToken(token),
		).Scan(&userID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}

		// Use the same user-before-token lock order as token creation.
		var lockedUserID int
		err = tx.QueryRowContext(ctx,
			"SELECT id FROM users WHERE id = $1 AND deleted_at IS NULL FOR UPDATE", userID,
		).Scan(&lockedUserID)
		if errors.Is(err, sql.ErrNoRows) {
			return errResetTargetMissing
		}
		if err != nil {
			return err
		}

		err = tx.QueryRowContext(ctx,
			`UPDATE password_reset_tokens
			 SET used_at = now()
			 WHERE token_hash = $1 AND user_id = $2 AND used_at IS NULL AND expires_at > now()
			 RETURNING user_id`,
			hashOpaqueToken(token), lockedUserID,
		).Scan(&userID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}

		result, err := tx.ExecContext(ctx,
			"UPDATE users SET password_hash = $1 WHERE id = $2 AND deleted_at IS NULL",
			passwordHash, userID,
		)
		if err != nil {
			return err
		}
		affected, err := rowsAffected(result)
		if err != nil {
			return err
		}
		if affected == 0 {
			return errResetTargetMissing
		}

		if _, err := tx.ExecContext(ctx, "DELETE FROM sessions WHERE user_id = $1", userID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM password_reset_email_outbox WHERE user_id = $1", userID); err != nil {
			return err
		}
		used = true
		return nil
	})
	if errors.Is(err, errResetTargetMissing) {
		return false, nil
	}
	return used, err
}

// errResetTargetMissing rolls back token consumption when the user is gone.
var errResetTargetMissing = errors.New("password reset user missing")

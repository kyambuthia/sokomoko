package db

import (
	"database/sql"
	"errors"
	"time"
)

func (s *Store) CreateSession(sess Session) error {
	ctx, cancel := s.ctx()
	defer cancel()
	_, err := s.DB.ExecContext(ctx,
		"INSERT INTO sessions (id, user_id, csrf_token, expires_at) VALUES ($1, $2, $3, $4)",
		hashOpaqueToken(sess.ID), sess.UserID, sess.CSRFToken, sess.ExpiresAt.UTC(),
	)
	return wrapDBError(err)
}

// GetSession returns the unexpired session for a raw cookie token.
func (s *Store) GetSession(id string) (*Session, error) {
	ctx, cancel := s.ctx()
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

func (s *Store) DeleteSession(id string) error {
	ctx, cancel := s.ctx()
	defer cancel()
	_, err := s.DB.ExecContext(ctx, "DELETE FROM sessions WHERE id = $1", hashOpaqueToken(id))
	return err
}

func (s *Store) DeleteSessionsByUserID(userID int) error {
	ctx, cancel := s.ctx()
	defer cancel()
	_, err := s.DB.ExecContext(ctx, "DELETE FROM sessions WHERE user_id = $1", userID)
	return err
}

func (s *Store) CleanupSessions() error {
	ctx, cancel := s.ctx()
	defer cancel()
	_, err := s.DB.ExecContext(ctx, "DELETE FROM sessions WHERE expires_at <= now()")
	return err
}

func (s *Store) CountActiveSessions() (int, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	var count int
	err := s.DB.QueryRowContext(ctx, "SELECT count(*) FROM sessions WHERE expires_at > now()").Scan(&count)
	return count, err
}

func (s *Store) CleanupPasswordResetTokens() error {
	ctx, cancel := s.ctx()
	defer cancel()
	_, err := s.DB.ExecContext(ctx,
		"DELETE FROM password_reset_tokens WHERE used_at IS NOT NULL OR expires_at <= now()",
	)
	return err
}

// CreatePasswordResetToken stores a new reset token, invalidating any earlier
// tokens for the same user.
func (s *Store) CreatePasswordResetToken(userID int, token string, expiresAt time.Time) error {
	ctx, cancel := s.ctx()
	defer cancel()

	return s.withTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "DELETE FROM password_reset_tokens WHERE user_id = $1", userID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx,
			"INSERT INTO password_reset_tokens (token_hash, user_id, expires_at) VALUES ($1, $2, $3)",
			hashOpaqueToken(token), userID, expiresAt.UTC(),
		)
		return err
	})
}

func (s *Store) GetValidPasswordResetToken(token string) (*PasswordResetToken, error) {
	ctx, cancel := s.ctx()
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
func (s *Store) UsePasswordResetToken(token, passwordHash string) (bool, error) {
	ctx, cancel := s.ctx()
	defer cancel()

	used := false
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		used = false
		var userID int
		err := tx.QueryRowContext(ctx,
			`UPDATE password_reset_tokens
			 SET used_at = now()
			 WHERE token_hash = $1 AND used_at IS NULL AND expires_at > now()
			 RETURNING user_id`,
			hashOpaqueToken(token),
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

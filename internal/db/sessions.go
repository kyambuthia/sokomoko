package db

import (
	"database/sql"
	"time"
)

func (s *Store) CreateSession(sess Session) error {
	stmt, err := s.DB.Prepare("INSERT INTO sessions (id, user_id, expires_at) VALUES (?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(hashSessionID(sess.ID), sess.UserID, sess.ExpiresAt)
	return err
}

func (s *Store) GetSession(id string) (*Session, error) {
	row := s.DB.QueryRow("SELECT id, user_id, expires_at, created_at FROM sessions WHERE id = ?", hashSessionID(id))
	sess := &Session{}
	err := row.Scan(&sess.ID, &sess.UserID, &sess.ExpiresAt, &sess.CreatedAt)
	if err == sql.ErrNoRows {
		row = s.DB.QueryRow("SELECT id, user_id, expires_at, created_at FROM sessions WHERE id = ?", id)
		err = row.Scan(&sess.ID, &sess.UserID, &sess.ExpiresAt, &sess.CreatedAt)
		if err == sql.ErrNoRows {
			return nil, nil
		}
	}
	if err != nil {
		return nil, err
	}
	return sess, nil
}

func (s *Store) DeleteSession(id string) error {
	_, err := s.DB.Exec("DELETE FROM sessions WHERE id = ? OR id = ?", hashSessionID(id), id)
	return err
}

func (s *Store) DeleteSessionsByUserID(userID int) error {
	_, err := s.DB.Exec("DELETE FROM sessions WHERE user_id = ?", userID)
	return err
}

func (s *Store) CleanupSessions() error {
	_, err := s.DB.Exec("DELETE FROM sessions WHERE expires_at < CURRENT_TIMESTAMP")
	return err
}

func (s *Store) CleanupPasswordResetTokens() error {
	_, err := s.DB.Exec(
		"DELETE FROM password_reset_tokens WHERE used_at IS NOT NULL OR expires_at < CURRENT_TIMESTAMP",
	)
	return err
}

func (s *Store) CreatePasswordResetToken(userID int, token string, expiresAt time.Time) error {
	stmt, err := s.DB.Prepare("INSERT INTO password_reset_tokens (token_hash, user_id, expires_at) VALUES (?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(hashOpaqueToken(token), userID, expiresAt)
	return err
}

func (s *Store) GetValidPasswordResetToken(token string) (*PasswordResetToken, error) {
	row := s.DB.QueryRow(
		`SELECT id, token_hash, user_id, expires_at, used_at, created_at
		 FROM password_reset_tokens
		 WHERE token_hash = ? AND used_at IS NULL AND expires_at > CURRENT_TIMESTAMP`,
		hashOpaqueToken(token),
	)

	resetToken := &PasswordResetToken{}
	err := row.Scan(
		&resetToken.ID,
		&resetToken.TokenHash,
		&resetToken.UserID,
		&resetToken.ExpiresAt,
		&resetToken.UsedAt,
		&resetToken.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return resetToken, nil
}

func (s *Store) UsePasswordResetToken(token, passwordHash, salt string) (bool, error) {
	tokenHash := hashOpaqueToken(token)
	tx, err := s.DB.Begin()
	if err != nil {
		return false, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var userID int
	var expiresAt time.Time
	var usedAt sql.NullTime
	err = tx.QueryRow(
		"SELECT user_id, expires_at, used_at FROM password_reset_tokens WHERE token_hash = ?",
		tokenHash,
	).Scan(&userID, &expiresAt, &usedAt)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	if usedAt.Valid || expiresAt.Before(time.Now()) {
		return false, nil
	}

	result, err := tx.Exec(
		"UPDATE users SET password_hash = ?, salt = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND deleted_at IS NULL",
		passwordHash, salt, userID,
	)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected == 0 {
		return false, nil
	}

	_, err = tx.Exec(
		"UPDATE password_reset_tokens SET used_at = CURRENT_TIMESTAMP WHERE token_hash = ? AND used_at IS NULL",
		tokenHash,
	)
	if err != nil {
		return false, err
	}

	_, err = tx.Exec("DELETE FROM sessions WHERE user_id = ?", userID)
	if err != nil {
		return false, err
	}

	if err = tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

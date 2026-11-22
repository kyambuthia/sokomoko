package db

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// APIToken represents a persisted bearer API token (without the raw secret).
type APIToken struct {
	ID          int64
	UserID      int
	Name        string
	TokenPrefix string
	ClientType  string
	LastUsedAt  sql.NullTime
	ExpiresAt   sql.NullTime
	RevokedAt   sql.NullTime
	CreatedAt   time.Time
}

// CreateAPIToken stores a new hashed API token for an active user.
func (s *Store) CreateAPIToken(ctx context.Context, userID int, name, tokenPrefix, rawToken, clientType string, expiresAt sql.NullTime) (*APIToken, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()

	name = strings.TrimSpace(name)
	tokenPrefix = strings.TrimSpace(tokenPrefix)
	clientType = strings.TrimSpace(clientType)
	if clientType == "" {
		clientType = "agent"
	}

	var exp any
	if expiresAt.Valid {
		exp = expiresAt.Time.UTC()
	}

	tok := &APIToken{}
	err := s.DB.QueryRowContext(ctx,
		`INSERT INTO api_tokens (user_id, name, token_prefix, token_hash, client_type, expires_at)
		 SELECT u.id, $2, $3, $4, $5, $6
		 FROM users u
		 WHERE u.id = $1 AND u.deleted_at IS NULL
		 RETURNING id, user_id, name, token_prefix, client_type, last_used_at, expires_at, revoked_at, created_at`,
		userID, name, tokenPrefix, hashOpaqueToken(rawToken), clientType, exp,
	).Scan(
		&tok.ID,
		&tok.UserID,
		&tok.Name,
		&tok.TokenPrefix,
		&tok.ClientType,
		&tok.LastUsedAt,
		&tok.ExpiresAt,
		&tok.RevokedAt,
		&tok.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserInactive
	}
	if err != nil {
		return nil, err
	}
	return tok, nil
}

// ListAPITokensByUser returns all API tokens belonging to userID, newest first.
func (s *Store) ListAPITokensByUser(ctx context.Context, userID int) ([]APIToken, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()

	rows, err := s.DB.QueryContext(ctx,
		`SELECT id, user_id, name, token_prefix, client_type, last_used_at, expires_at, revoked_at, created_at
		 FROM api_tokens
		 WHERE user_id = $1
		 ORDER BY created_at DESC, id DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []APIToken
	for rows.Next() {
		var tok APIToken
		if err := rows.Scan(
			&tok.ID,
			&tok.UserID,
			&tok.Name,
			&tok.TokenPrefix,
			&tok.ClientType,
			&tok.LastUsedAt,
			&tok.ExpiresAt,
			&tok.RevokedAt,
			&tok.CreatedAt,
		); err != nil {
			return nil, err
		}
		tokens = append(tokens, tok)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return tokens, nil
}

// LookupActiveAPITokenUser resolves the active user and token record for a raw
// bearer token, rejecting revoked, expired, deleted, or suspended-partner accounts.
func (s *Store) LookupActiveAPITokenUser(ctx context.Context, rawToken string) (*User, *APIToken, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()

	user := &User{}
	tok := &APIToken{}
	err := s.DB.QueryRowContext(ctx,
		`SELECT u.id, u.username, u.email, u.password_hash, u.role,
		        u.created_at, u.updated_at, u.deleted_at,
		        t.id, t.user_id, t.name, t.token_prefix, t.client_type,
		        t.last_used_at, t.expires_at, t.revoked_at, t.created_at
		 FROM api_tokens t
		 JOIN users u ON u.id = t.user_id
		 WHERE t.token_hash = $1
		   AND t.revoked_at IS NULL
		   AND (t.expires_at IS NULL OR t.expires_at > now())
		   AND u.deleted_at IS NULL
		   AND (
		     u.role <> 'partner'
		     OR EXISTS (
		       SELECT 1
		       FROM partner_users pu
		       JOIN partners p ON p.id = pu.partner_id
		       WHERE pu.user_id = u.id AND p.status = 'active'
		     )
		   )`,
		hashOpaqueToken(rawToken),
	).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.PasswordHash,
		&user.Role,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.DeletedAt,
		&tok.ID,
		&tok.UserID,
		&tok.Name,
		&tok.TokenPrefix,
		&tok.ClientType,
		&tok.LastUsedAt,
		&tok.ExpiresAt,
		&tok.RevokedAt,
		&tok.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	return user, tok, nil
}

// RevokeAPIToken marks a token as revoked if it belongs to userID and is not
// already revoked.
func (s *Store) RevokeAPIToken(ctx context.Context, userID int, tokenID int64) (bool, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()

	res, err := s.DB.ExecContext(ctx,
		`UPDATE api_tokens
		 SET revoked_at = now()
		 WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL`,
		tokenID, userID,
	)
	if err != nil {
		return false, err
	}
	affected, err := rowsAffected(res)
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

// TouchAPITokenLastUsed updates last_used_at for an active token.
func (s *Store) TouchAPITokenLastUsed(ctx context.Context, tokenID int64) error {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()

	_, err := s.DB.ExecContext(ctx,
		`UPDATE api_tokens SET last_used_at = now() WHERE id = $1 AND revoked_at IS NULL`,
		tokenID,
	)
	return err
}

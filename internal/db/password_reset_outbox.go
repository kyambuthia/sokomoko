package db

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

const (
	maxPasswordResetEmailOutboxClaim = 100
	minPasswordResetEmailOutboxLease = time.Second
	maxPasswordResetEmailOutboxLease = 5 * time.Minute
)

var (
	ErrInvalidPasswordResetEmailOutboxClaim = errors.New("invalid password reset email outbox claim parameters")
	ErrPasswordResetEmailOutboxLeaseLost    = errors.New("password reset email outbox lease lost")
)

// PasswordResetEmailOutboxEntry contains only sealed recipient and link data.
type PasswordResetEmailOutboxEntry struct {
	ID        int64
	UserID    int
	Payload   []byte
	ExpiresAt time.Time
	Attempts  int
}

// ClaimPasswordResetEmails leases due, unexpired messages to this worker.
// SKIP LOCKED lets independent application replicas drain the same queue.
func (s *Store) ClaimPasswordResetEmails(ctx context.Context, limit int, lease time.Duration) ([]PasswordResetEmailOutboxEntry, error) {
	if limit < 1 || limit > maxPasswordResetEmailOutboxClaim || lease < minPasswordResetEmailOutboxLease || lease > maxPasswordResetEmailOutboxLease {
		return nil, ErrInvalidPasswordResetEmailOutboxClaim
	}
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	entries := make([]PasswordResetEmailOutboxEntry, 0, limit)
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
			WITH due AS (
				SELECT id
				FROM password_reset_email_outbox
				WHERE available_at <= clock_timestamp()
				  AND expires_at > clock_timestamp()
				  AND (lease_until IS NULL OR lease_until <= clock_timestamp())
				ORDER BY available_at, id
				FOR UPDATE SKIP LOCKED
				LIMIT $1
			)
			UPDATE password_reset_email_outbox AS mail
			SET lease_until = clock_timestamp() + ($2::double precision * interval '1 microsecond'),
			    attempts = mail.attempts + 1
			FROM due
			WHERE mail.id = due.id
			RETURNING mail.id, mail.user_id, mail.sealed_payload, mail.expires_at, mail.attempts`,
			limit, lease.Microseconds(),
		)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			entry := PasswordResetEmailOutboxEntry{}
			if err := rows.Scan(&entry.ID, &entry.UserID, &entry.Payload, &entry.ExpiresAt, &entry.Attempts); err != nil {
				return err
			}
			entries = append(entries, entry)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}

// MarkPasswordResetEmailSent removes a successfully delivered message.
func (s *Store) MarkPasswordResetEmailSent(ctx context.Context, id int64, attempts int) error {
	return s.deletePasswordResetEmailOutboxEntry(ctx, id, attempts)
}

// RenewPasswordResetEmailLease extends the worker lease while the message is actively being processed.
func (s *Store) RenewPasswordResetEmailLease(ctx context.Context, id int64, attempts int, lease time.Duration) error {
	if lease < minPasswordResetEmailOutboxLease || lease > maxPasswordResetEmailOutboxLease {
		return ErrInvalidPasswordResetEmailOutboxClaim
	}
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	res, err := s.DB.ExecContext(ctx, `
		UPDATE password_reset_email_outbox
		SET lease_until = clock_timestamp() + ($3::double precision * interval '1 microsecond')
		WHERE id = $1 AND attempts = $2 AND lease_until > clock_timestamp() AND expires_at > clock_timestamp()`,
		id, attempts, lease.Microseconds(),
	)
	if err != nil {
		return err
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrPasswordResetEmailOutboxLeaseLost
	}
	return nil
}

// RetryPasswordResetEmail releases a lease and schedules the next delivery
// attempt, capped at token expiry.
func (s *Store) RetryPasswordResetEmail(ctx context.Context, id int64, attempts int, availableAt time.Time) error {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	res, err := s.DB.ExecContext(ctx, `
		UPDATE password_reset_email_outbox
		SET available_at = LEAST($3, expires_at), lease_until = NULL
		WHERE id = $1 AND attempts = $2 AND lease_until > clock_timestamp() AND expires_at > clock_timestamp()`, id, attempts, availableAt.UTC())
	if err != nil {
		return err
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrPasswordResetEmailOutboxLeaseLost
	}
	return nil
}

// DiscardPasswordResetEmail removes an expired or permanently undeliverable message.
func (s *Store) DiscardPasswordResetEmail(ctx context.Context, id int64, attempts int) error {
	return s.deletePasswordResetEmailOutboxEntry(ctx, id, attempts)
}

func (s *Store) deletePasswordResetEmailOutboxEntry(ctx context.Context, id int64, attempts int) error {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	res, err := s.DB.ExecContext(ctx, "DELETE FROM password_reset_email_outbox WHERE id = $1 AND attempts = $2 AND lease_until > clock_timestamp()", id, attempts)
	if err != nil {
		return err
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrPasswordResetEmailOutboxLeaseLost
	}
	return nil
}

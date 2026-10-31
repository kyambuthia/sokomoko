package auth

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/kyambuthia/sokomoko/internal/db"
)

const (
	resetEmailOutboxPollInterval = time.Second
	resetEmailOutboxLease        = 30 * time.Second
	resetEmailOutboxRetryBase    = 5 * time.Second
	resetEmailOutboxRetryMax     = 5 * time.Minute
	resetEmailOutboxDBTimeout    = 3 * time.Second
)

type passwordResetOutboxPayload struct {
	UserID         int    `json:"user_id"`
	RecipientEmail string `json:"recipient_email"`
	ResetLink      string `json:"reset_link"`
}

type passwordResetOutboxStore interface {
	RequestPasswordResetTokenWithOutbox(ctx context.Context, userID int, token string, expiresAt time.Time, sealedPayload []byte) (bool, error)
	ClaimPasswordResetEmails(ctx context.Context, limit int, lease time.Duration) ([]db.PasswordResetEmailOutboxEntry, error)
	RenewPasswordResetEmailLease(ctx context.Context, id int64, attempts int, lease time.Duration) error
	MarkPasswordResetEmailSent(ctx context.Context, id int64, attempts int) error
	RetryPasswordResetEmail(ctx context.Context, id int64, attempts int, availableAt time.Time) error
	DiscardPasswordResetEmail(ctx context.Context, id int64, attempts int) error
}

func (s *Service) requestPasswordResetToken(ctx context.Context, userID int, token string, expiresAt time.Time, recipient, resetLink string) (created bool, queued bool, err error) {
	if s.passwordResetEmailSender != nil && s.passwordResetOutboxBox != nil {
		if outbox, ok := s.store.(passwordResetOutboxStore); ok {
			payload, err := json.Marshal(passwordResetOutboxPayload{UserID: userID, RecipientEmail: recipient, ResetLink: resetLink})
			if err != nil {
				return false, true, err
			}
			sealed, err := s.passwordResetOutboxBox.Seal(payload)
			if err != nil {
				return false, true, err
			}
			created, err := outbox.RequestPasswordResetTokenWithOutbox(ctx, userID, token, expiresAt, sealed)
			return created, true, err
		}
	}
	created, err = s.store.RequestPasswordResetToken(ctx, userID, token, expiresAt)
	return created, false, err
}

// MaintainPasswordResetEmailOutbox drains durable reset mail until shutdown.
// PostgreSQL leases keep only one replica responsible for each message.
func (s *Service) MaintainPasswordResetEmailOutbox(ctx context.Context) {
	outbox, ok := s.store.(passwordResetOutboxStore)
	if !ok || s.passwordResetEmailSender == nil || s.passwordResetOutboxBox == nil {
		return
	}
	poll := time.NewTicker(resetEmailOutboxPollInterval)
	defer poll.Stop()
	for ctx.Err() == nil {
		entries, err := outbox.ClaimPasswordResetEmails(ctx, 1, resetEmailOutboxLease)
		if err != nil {
			if ctx.Err() == nil {
				slog.Error("password reset email outbox claim failed", "error", err)
			}
			if !waitForResetEmailOutbox(ctx, poll.C) {
				return
			}
			continue
		}
		if len(entries) == 0 {
			if !waitForResetEmailOutbox(ctx, poll.C) {
				return
			}
			continue
		}
		s.deliverPasswordResetOutboxEntry(ctx, outbox, entries[0])
	}
}

func (s *Service) deliverPasswordResetOutboxEntry(ctx context.Context, outbox passwordResetOutboxStore, entry db.PasswordResetEmailOutboxEntry) {
	finish := func(operation func(context.Context) error) error {
		opCtx := ctx
		if ctx.Err() != nil {
			opCtx = context.WithoutCancel(ctx)
		}
		opCtx, cancel := context.WithTimeout(opCtx, resetEmailOutboxDBTimeout)
		defer cancel()
		return operation(opCtx)
	}
	discard := func() {
		if err := finish(func(ctx context.Context) error {
			return outbox.DiscardPasswordResetEmail(ctx, entry.ID, entry.Attempts)
		}); err != nil && ctx.Err() == nil {
			if errors.Is(err, db.ErrPasswordResetEmailOutboxLeaseLost) {
				return
			}
			slog.Error("password reset email outbox discard failed", "user_id", entry.UserID, "error", err)
		}
	}
	if !time.Now().Before(entry.ExpiresAt) {
		discard()
		return
	}
	plaintext, err := s.passwordResetOutboxBox.Open(entry.Payload)
	if err != nil {
		slog.Error("password reset email outbox payload cannot be decrypted", "user_id", entry.UserID)
		discard()
		return
	}
	var payload passwordResetOutboxPayload
	if err := json.Unmarshal(plaintext, &payload); err != nil || payload.UserID != entry.UserID || payload.RecipientEmail == "" || payload.ResetLink == "" {
		slog.Error("password reset email outbox payload is invalid", "user_id", entry.UserID)
		discard()
		return
	}
	if ctx.Err() != nil {
		s.reschedulePasswordResetOutboxEntry(ctx, outbox, entry, time.Now())
		return
	}
	if err := finish(func(ctx context.Context) error {
		return outbox.RenewPasswordResetEmailLease(ctx, entry.ID, entry.Attempts, resetEmailOutboxLease)
	}); err != nil {
		if errors.Is(err, db.ErrPasswordResetEmailOutboxLeaseLost) {
			return
		}
		slog.Error("password reset email outbox lease renewal failed", "user_id", entry.UserID, "error", err)
		return
	}
	sendCtx, cancel := context.WithTimeout(ctx, resetEmailTimeout)
	err = s.passwordResetEmailSender(sendCtx, payload.RecipientEmail, payload.ResetLink)
	cancel()
	if err == nil {
		if markErr := finish(func(ctx context.Context) error {
			return outbox.MarkPasswordResetEmailSent(ctx, entry.ID, entry.Attempts)
		}); markErr != nil {
			if errors.Is(markErr, db.ErrPasswordResetEmailOutboxLeaseLost) {
				return
			}
			slog.Error("password reset email outbox completion failed", "user_id", entry.UserID, "error", markErr)
		}
		return
	}
	if ctx.Err() != nil {
		s.reschedulePasswordResetOutboxEntry(ctx, outbox, entry, time.Now())
		return
	}
	if permanentMailError(err) {
		slog.Error("password reset email rejected by SMTP", "user_id", entry.UserID, "error", err)
		discard()
		return
	}
	nextAttempt := time.Now().Add(resetEmailOutboxRetryDelay(entry.Attempts))
	if !nextAttempt.Before(entry.ExpiresAt) {
		discard()
		return
	}
	s.reschedulePasswordResetOutboxEntry(ctx, outbox, entry, nextAttempt)
}

func (s *Service) reschedulePasswordResetOutboxEntry(ctx context.Context, outbox passwordResetOutboxStore, entry db.PasswordResetEmailOutboxEntry, availableAt time.Time) {
	opCtx := ctx
	if ctx.Err() != nil {
		opCtx = context.WithoutCancel(ctx)
	}
	opCtx, cancel := context.WithTimeout(opCtx, resetEmailOutboxDBTimeout)
	defer cancel()
	if err := outbox.RetryPasswordResetEmail(opCtx, entry.ID, entry.Attempts, availableAt); err != nil && ctx.Err() == nil {
		if errors.Is(err, db.ErrPasswordResetEmailOutboxLeaseLost) {
			return
		}
		slog.Error("password reset email outbox reschedule failed", "user_id", entry.UserID, "error", err)
	}
}

func resetEmailOutboxRetryDelay(attempt int) time.Duration {
	delay := resetEmailOutboxRetryBase
	for n := 1; n < attempt && delay < resetEmailOutboxRetryMax; n++ {
		delay *= 2
	}
	if delay > resetEmailOutboxRetryMax {
		return resetEmailOutboxRetryMax
	}
	return delay
}

func waitForResetEmailOutbox(ctx context.Context, tick <-chan time.Time) bool {
	select {
	case <-ctx.Done():
		return false
	case <-tick:
		return true
	}
}

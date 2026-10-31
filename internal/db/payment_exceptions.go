package db

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/kyambuthia/sokomoko/internal/money"
)

var ErrPaymentExceptionNotFound = errors.New("payment exception not found")

// PaymentException is one confirmed charge that needs an operator refund.
type PaymentException struct {
	ID               int
	PaymentID        int
	AttemptID        int
	OrderID          int
	Reason           string
	Status           string
	Provider         string
	Currency         string
	Amount           money.Cents
	RequestReference string
	Receipt          string
	RefundReference  string
	LastError        string
	CreatedAt        time.Time
}

const paymentExceptionSelect = `SELECT e.id, e.payment_id, e.attempt_id, p.order_id,
    e.reason, e.status, e.provider, e.currency, e.amount_cents,
    e.request_reference, e.receipt, e.refund_reference, e.last_error, e.created_at
    FROM payment_exceptions e JOIN payments p ON p.id = e.payment_id`

func scanPaymentException(row interface{ Scan(...any) error }) (*PaymentException, error) {
	var e PaymentException
	var amount int64
	err := row.Scan(&e.ID, &e.PaymentID, &e.AttemptID, &e.OrderID,
		&e.Reason, &e.Status, &e.Provider, &e.Currency, &amount,
		&e.RequestReference, &e.Receipt, &e.RefundReference, &e.LastError, &e.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrPaymentExceptionNotFound
	}
	if err != nil {
		return nil, err
	}
	e.Amount = money.Cents(amount)
	return &e, nil
}

func (s *Store) ListPaymentExceptions(ctx context.Context, limit int) ([]PaymentException, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	// Limit the ordered exception set before joining payment details.
	rows, err := s.DB.QueryContext(ctx, `SELECT e.id,e.payment_id,e.attempt_id,p.order_id,
    e.reason,e.status,e.provider,e.currency,e.amount_cents,
    e.request_reference,e.receipt,e.refund_reference,e.last_error,e.created_at
    FROM (
        SELECT id,payment_id,attempt_id,reason,status,provider,currency,amount_cents,
               request_reference,receipt,refund_reference,last_error,created_at
        FROM payment_exceptions
        ORDER BY (status = 'refunded'), created_at DESC, id DESC
        LIMIT $1
    ) e
    JOIN payments p ON p.id=e.payment_id
    ORDER BY (e.status = 'refunded'), e.created_at DESC, e.id DESC`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []PaymentException{}
	for rows.Next() {
		e, err := scanPaymentException(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *e)
	}
	return result, rows.Err()
}

func (s *Store) GetPaymentException(ctx context.Context, id int) (*PaymentException, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	return scanPaymentException(s.DB.QueryRowContext(ctx, paymentExceptionSelect+" WHERE e.id = $1", id))
}

// BeginRefund claims an open exception before any provider request for a
// trusted internal caller. Repeated submissions never send a second refund,
// including after an unknown timeout.
func (s *Store) BeginRefund(ctx context.Context, id int) (*PaymentException, bool, error) {
	return s.beginRefund(ctx, 0, id, false)
}

// BeginRefundForActor revalidates the admin in the claim transaction.
func (s *Store) BeginRefundForActor(ctx context.Context, actorID, id int) (*PaymentException, bool, error) {
	return s.beginRefund(ctx, actorID, id, true)
}

func (s *Store) beginRefund(ctx context.Context, actorID, id int, requireAdmin bool) (*PaymentException, bool, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	claimed := false
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		if requireAdmin {
			if _, _, err := lockWorkspaceActorTx(ctx, tx, actorID, RoleAdmin); err != nil {
				return err
			}
		}
		var status string
		if err := tx.QueryRowContext(ctx, "SELECT status FROM payment_exceptions WHERE id = $1 FOR UPDATE", id).Scan(&status); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrPaymentExceptionNotFound
			}
			return err
		}
		if status != "open" {
			return nil
		}
		if _, err := tx.ExecContext(ctx, "UPDATE payment_exceptions SET status = 'requested' WHERE id = $1", id); err != nil {
			return err
		}
		claimed = true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	e, err := s.GetPaymentException(ctx, id)
	return e, claimed, err
}

func (s *Store) SetPaymentExceptionStatus(ctx context.Context, id int, status, refundReference, lastError string) error {
	return s.setPaymentExceptionStatus(ctx, 0, id, status, refundReference, lastError, false)
}

// SetPaymentExceptionStatusForActor revalidates an admin for manual resolution.
// Provider-result persistence continues through the trusted method.
func (s *Store) SetPaymentExceptionStatusForActor(ctx context.Context, actorID, id int, status, refundReference, lastError string) error {
	return s.setPaymentExceptionStatus(ctx, actorID, id, status, refundReference, lastError, true)
}

func (s *Store) setPaymentExceptionStatus(ctx context.Context, actorID, id int, status, refundReference, lastError string, requireAdmin bool) error {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	return s.withTx(ctx, func(tx *sql.Tx) error {
		if requireAdmin {
			if _, _, err := lockWorkspaceActorTx(ctx, tx, actorID, RoleAdmin); err != nil {
				return err
			}
		}
		var paymentID int
		var reason, current string
		if err := tx.QueryRowContext(ctx,
			"SELECT payment_id, reason, status FROM payment_exceptions WHERE id = $1 FOR UPDATE", id).
			Scan(&paymentID, &reason, &current); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrPaymentExceptionNotFound
			}
			return err
		}
		if current == "refunded" {
			return nil
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE payment_exceptions SET status = $2,
			 refund_reference = COALESCE(NULLIF($3, ''), refund_reference), last_error = $4
			 WHERE id = $1`, id, status, refundReference, truncateText(lastError, 500)); err != nil {
			return err
		}
		if status == "refunded" && reason == "late_capture" {
			_, err := tx.ExecContext(ctx,
				"UPDATE payments SET status = 'refunded' WHERE id = $1 AND status = 'captured'", paymentID)
			return err
		}
		return nil
	})
}

// RecordMpesaRefundForActor revalidates the admin and marks an eligible M-Pesa
// exception refunded, recording the reversal receipt in one transaction.
func (s *Store) RecordMpesaRefundForActor(ctx context.Context, actorID, id int, receipt string) error {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	return s.withTx(ctx, func(tx *sql.Tx) error {
		if _, _, err := lockWorkspaceActorTx(ctx, tx, actorID, RoleAdmin); err != nil {
			return err
		}
		var paymentID int
		var reason, status, provider string
		if err := tx.QueryRowContext(ctx,
			"SELECT payment_id, reason, status, provider FROM payment_exceptions WHERE id = $1 FOR UPDATE", id).
			Scan(&paymentID, &reason, &status, &provider); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrPaymentExceptionNotFound
			}
			return err
		}
		if status == "refunded" {
			return nil
		}
		if provider != "mpesa" || (status != "open" && status != "requested" && status != "review") {
			return nil
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE payment_exceptions SET status = 'refunded',
			 refund_reference = COALESCE(NULLIF($2, ''), refund_reference), last_error = ''
			 WHERE id = $1`, id, receipt); err != nil {
			return err
		}
		if reason == "late_capture" {
			_, err := tx.ExecContext(ctx,
				"UPDATE payments SET status = 'refunded' WHERE id = $1 AND status = 'captured'", paymentID)
			return err
		}
		return nil
	})
}

// PaymentExceptionCounts is a small health signal for operators.
func (s *Store) PaymentExceptionCounts(ctx context.Context) (open, pending, review int, err error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	err = s.DB.QueryRowContext(ctx,
		`SELECT count(*) FILTER (WHERE status = 'open'),
		        count(*) FILTER (WHERE status IN ('requested', 'pending')),
		        count(*) FILTER (WHERE status = 'review')
		 FROM payment_exceptions`).Scan(&open, &pending, &review)
	return
}

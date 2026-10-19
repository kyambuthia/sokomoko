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
	rows, err := s.DB.QueryContext(ctx, paymentExceptionSelect+
		" ORDER BY (e.status = 'refunded'), e.created_at DESC, e.id DESC LIMIT $1", limit)
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

// BeginRefund claims an open exception before any provider request. Repeated
// submissions never send a second refund, including after an unknown timeout.
func (s *Store) BeginRefund(ctx context.Context, id int) (*PaymentException, bool, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	claimed := false
	err := s.withTx(ctx, func(tx *sql.Tx) error {
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
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	return s.withTx(ctx, func(tx *sql.Tx) error {
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

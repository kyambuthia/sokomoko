package db

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/kyambuthia/sokomoko/internal/domain"
	"github.com/kyambuthia/sokomoko/internal/money"
)

// Paystack's HTTP client times out after 20 seconds; wait a minute before
// allowing a manual resolution to race an uncertain provider request.
const paymentRefundResolveGrace = time.Minute

var ErrPaymentExceptionNotFound = domain.ErrPaymentExceptionNotFound

// PaymentException is one confirmed charge that needs an operator refund.
type PaymentException = domain.PaymentException

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
		if status == "refunded" {
			if err := tx.QueryRowContext(ctx, "SELECT payment_id FROM payment_exceptions WHERE id = $1", id).Scan(&paymentID); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return ErrPaymentExceptionNotFound
				}
				return err
			}
			// CapturePayment locks the payment before creating its exception.
			// Keep refund completion on the same payment-before-exception order.
			var lockedPaymentID int
			if err := tx.QueryRowContext(ctx, "SELECT id FROM payments WHERE id = $1 FOR UPDATE", paymentID).Scan(&lockedPaymentID); err != nil {
				return err
			}
		}
		var exceptionPaymentID int
		var reason, current string
		var providerRequestInProgress, affectsPayment bool
		if err := tx.QueryRowContext(ctx,
			`SELECT payment_id, reason, status,
       status IN ('requested', 'review') AND provider = 'paystack'
           AND updated_at > clock_timestamp() - ($2::double precision * interval '1 microsecond'),
       affects_payment
FROM payment_exceptions
WHERE id = $1
FOR UPDATE`,
			id, paymentRefundResolveGrace.Microseconds()).
			Scan(&exceptionPaymentID, &reason, &current, &providerRequestInProgress, &affectsPayment); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrPaymentExceptionNotFound
			}
			return err
		}
		if status == "refunded" && paymentID != exceptionPaymentID {
			return ErrPaymentExceptionNotFound
		}
		paymentID = exceptionPaymentID
		if current == "refunded" {
			return nil
		}
		if requireAdmin && status == "refunded" && providerRequestInProgress {
			return ErrPaymentRefundRequestInProgress
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE payment_exceptions SET status = $2,
			 refund_reference = COALESCE(NULLIF($3, ''), refund_reference), last_error = $4
			 WHERE id = $1`, id, status, refundReference, truncateText(lastError, 500)); err != nil {
			return err
		}
		if status == "refunded" && (reason == "late_capture" || affectsPayment) {
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
		if err := tx.QueryRowContext(ctx, "SELECT payment_id FROM payment_exceptions WHERE id = $1", id).Scan(&paymentID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrPaymentExceptionNotFound
			}
			return err
		}
		// Match the payment-before-exception lock order used by capture.
		var lockedPaymentID int
		if err := tx.QueryRowContext(ctx, "SELECT id FROM payments WHERE id = $1 FOR UPDATE", paymentID).Scan(&lockedPaymentID); err != nil {
			return err
		}
		var exceptionPaymentID int
		var reason, status, provider string
		var affectsPayment bool
		if err := tx.QueryRowContext(ctx,
			"SELECT payment_id, reason, status, provider, affects_payment FROM payment_exceptions WHERE id = $1 FOR UPDATE", id).
			Scan(&exceptionPaymentID, &reason, &status, &provider, &affectsPayment); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrPaymentExceptionNotFound
			}
			return err
		}
		if paymentID != exceptionPaymentID {
			return ErrPaymentExceptionNotFound
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
		if reason == "late_capture" || affectsPayment {
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

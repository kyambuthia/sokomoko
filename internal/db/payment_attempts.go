package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/kyambuthia/sokomoko/internal/money"
)

// OnlinePayment is the payment row an online provider works with.
type OnlinePayment struct {
	ID                int
	OrderID           int
	UserID            int
	UserEmail         string
	Method            string
	Provider          string
	Status            string
	Currency          string
	Amount            money.Cents
	ExternalReference string
	CallbackTokenHash string
	OrderStatus       string
	CreatedAt         time.Time
	AttemptID         int
	AttemptStatus     string
	ReviewRequired    bool
}

const onlinePaymentSelect = `
SELECT p.id, p.order_id, p.user_id, u.email, p.method, p.provider, p.status, p.currency, p.amount_cents,
       COALESCE(a.request_reference, ''), COALESCE(a.callback_token_hash, ''), o.status, p.created_at,
       COALESCE(a.id, 0), COALESCE(a.status, ''),
       EXISTS (
           SELECT 1 FROM payment_exceptions e
           WHERE e.payment_id = p.id
             AND e.reason IN ('amount_mismatch', 'amount_unverified')
             AND e.status <> 'refunded'
             AND (p.status <> 'captured' OR e.affects_payment)
       )
FROM payments p
JOIN users u ON u.id = p.user_id
JOIN orders o ON o.id = p.order_id`

const latestAttemptJoin = ` LEFT JOIN payment_attempts a ON a.id = (
    SELECT id FROM payment_attempts WHERE payment_id = p.id ORDER BY id DESC LIMIT 1)`

func scanOnlinePayment(row interface{ Scan(...any) error }) (*OnlinePayment, error) {
	var p OnlinePayment
	var amount int64
	err := row.Scan(&p.ID, &p.OrderID, &p.UserID, &p.UserEmail, &p.Method, &p.Provider, &p.Status, &p.Currency, &amount,
		&p.ExternalReference, &p.CallbackTokenHash, &p.OrderStatus, &p.CreatedAt, &p.AttemptID, &p.AttemptStatus, &p.ReviewRequired)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrPaymentNotFound
	}
	if err != nil {
		return nil, err
	}
	p.Amount = money.Cents(amount)
	return &p, nil
}

// GetOrderPayment returns the newest payment of an order owned by userID.
func (s *Store) GetOrderPayment(ctx context.Context, userID, orderID int) (*OnlinePayment, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	return scanOnlinePayment(s.DB.QueryRowContext(ctx,
		onlinePaymentSelect+latestAttemptJoin+" WHERE p.order_id = $1 AND p.user_id = $2 ORDER BY p.id DESC LIMIT 1", orderID, userID))
}

func (s *Store) GetPaymentByReference(ctx context.Context, provider, reference string) (*OnlinePayment, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	return scanOnlinePayment(s.DB.QueryRowContext(ctx,
		onlinePaymentSelect+" JOIN payment_attempts a ON a.payment_id = p.id WHERE p.provider = $1 AND a.request_reference = $2", provider, reference))
}

// GetPaymentByCallbackToken finds the attempt that was given
// the callback token with this SHA-256 hash.
func (s *Store) GetPaymentByCallbackToken(ctx context.Context, provider, tokenHash string) (*OnlinePayment, error) {
	if strings.TrimSpace(tokenHash) == "" {
		return nil, ErrPaymentNotFound
	}
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	return scanOnlinePayment(s.DB.QueryRowContext(ctx,
		onlinePaymentSelect+" JOIN payment_attempts a ON a.payment_id = p.id WHERE p.provider = $1 AND a.callback_token_hash = $2", provider, tokenHash))
}

// StartPaymentAttempt records that a provider request is being made for a
// pending payment: the new external reference, the hashed callback token,
// and the provider payload (for example M-Pesa request IDs).
func (s *Store) StartPaymentAttempt(ctx context.Context, paymentID int, reference, callbackTokenHash string, payload map[string]string) (int, error) {
	if payload == nil {
		payload = map[string]string{} // a JSON null would turn later merges into arrays
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	var attemptID int
	err = s.withTx(ctx, func(tx *sql.Tx) error {
		var status, orderStatus string
		if err := tx.QueryRowContext(ctx, `SELECT p.status, o.status FROM payments p JOIN orders o ON o.id = p.order_id
			WHERE p.id = $1 FOR UPDATE OF p`, paymentID).Scan(&status, &orderStatus); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrPaymentNotFound
			}
			return err
		}
		if status != PaymentStatusPending || orderStatus == OrderStatusCancelled {
			return ErrPaymentNotFound
		}
		var reviewRequired bool
		if err := tx.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM payment_exceptions
				WHERE payment_id = $1
				  AND reason IN ('amount_mismatch', 'amount_unverified')
				  AND status <> 'refunded'
			)`, paymentID).Scan(&reviewRequired); err != nil {
			return err
		}
		if reviewRequired {
			return ErrPaymentReviewRequired
		}
		var active int
		err := tx.QueryRowContext(ctx, `SELECT id FROM payment_attempts WHERE payment_id = $1 AND status = 'pending'`, paymentID).Scan(&active)
		if err == nil {
			return ErrAttemptInProgress
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		result, err := tx.ExecContext(ctx,
			`UPDATE payments SET external_reference = $1, callback_token_hash = NULLIF($2, ''), provider_payload = $3
			 WHERE id = $4 AND status = 'pending'`,
			reference, callbackTokenHash, raw, paymentID)
		if err != nil {
			return wrapDBError(err)
		}
		if n, err := rowsAffected(result); err != nil || n == 0 {
			if err == nil {
				err = ErrPaymentNotFound
			}
			return err
		}
		return tx.QueryRowContext(ctx,
			`INSERT INTO payment_attempts (payment_id, status, request_reference, external_reference, callback_token_hash, provider_payload)
			 VALUES ($1, 'pending', $2, $2, NULLIF($3, ''), $4) RETURNING id`,
			paymentID, reference, callbackTokenHash, raw).Scan(&attemptID)
	})
	return attemptID, err
}

// UpdatePaymentPayload merges provider identifiers into the payment, for
// example the M-Pesa CheckoutRequestID returned after an STK push.
func (s *Store) UpdatePaymentPayload(ctx context.Context, attemptID int, payload map[string]string) error {
	if payload == nil {
		payload = map[string]string{}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	return s.withTx(ctx, func(tx *sql.Tx) error {
		// Lock the payment before its attempt to match capture and cancellation.
		var paymentID int
		if err := tx.QueryRowContext(ctx,
			`SELECT p.id FROM payments p
			 JOIN payment_attempts a ON a.payment_id = p.id
			 WHERE a.id = $1 FOR UPDATE OF p`, attemptID).Scan(&paymentID); err != nil {
			return err
		}
		var attemptPaymentID int
		if err := tx.QueryRowContext(ctx,
			`UPDATE payment_attempts SET provider_payload = provider_payload || $1::jsonb
			 WHERE id = $2 RETURNING payment_id`, raw, attemptID).Scan(&attemptPaymentID); err != nil {
			return err
		}
		if attemptPaymentID != paymentID {
			return ErrPaymentNotFound
		}
		_, err := tx.ExecContext(ctx,
			`UPDATE payments SET provider_payload = provider_payload || $1::jsonb WHERE id = $2
			 AND external_reference = (SELECT request_reference FROM payment_attempts WHERE id = $3)`,
			raw, paymentID, attemptID)
		return err
	})
}

// PaymentPayload returns the stored provider identifiers.
func (s *Store) PaymentPayload(ctx context.Context, attemptID int) (map[string]string, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	var raw []byte
	if err := s.DB.QueryRowContext(ctx, "SELECT provider_payload FROM payment_attempts WHERE id = $1", attemptID).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrPaymentNotFound
		}
		return nil, err
	}
	payload := map[string]string{}
	return payload, json.Unmarshal(raw, &payload)
}

// CapturePayment records a provider-confirmed charge. Repeated confirmation
// of the same attempt is harmless; a second charged attempt or a charge after
// cancellation enters the operator refund queue.
func (s *Store) CapturePayment(ctx context.Context, attemptID int, confirmed money.Cents, currency, receipt string) (bool, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	captured := false
	backfillExceptionReceipt := false
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		var paymentID, orderID int
		var status, cur, provider, reference, attemptStatus, paymentReference string
		var amount int64
		var payload []byte
		// Lock payment before attempt so callback and payload updates share one order.
		if err := tx.QueryRowContext(ctx, `SELECT p.id, p.order_id, p.status, p.currency, p.provider, p.amount_cents, COALESCE(p.external_reference, '')
		FROM payments p JOIN payment_attempts a ON a.payment_id = p.id
		WHERE a.id = $1 FOR UPDATE OF p`, attemptID).
			Scan(&paymentID, &orderID, &status, &cur, &provider, &amount, &paymentReference); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrPaymentNotFound
			}
			return err
		}
		if err := tx.QueryRowContext(ctx,
			`SELECT request_reference, status, provider_payload FROM payment_attempts
			 WHERE id = $1 AND payment_id = $2 FOR UPDATE`, attemptID, paymentID).
			Scan(&reference, &attemptStatus, &payload); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrPaymentNotFound
			}
			return err
		}
		if attemptStatus == PaymentAttemptStatusCompleted {
			if receipt == "" {
				return nil
			}
			var data map[string]string
			if err := json.Unmarshal(payload, &data); err != nil {
				return err
			}
			expected := money.Cents(amount)
			if charged, err := strconv.ParseInt(data["charged_cents"], 10, 64); err == nil && charged > 0 {
				expected = money.Cents(charged)
			}
			if confirmed != expected || !strings.EqualFold(cur, currency) {
				return ErrPaymentAmountMatch
			}
			if _, err := tx.ExecContext(ctx,
				`UPDATE payment_attempts SET external_reference = $2
				 WHERE id = $1 AND (external_reference IS NULL OR external_reference = '' OR external_reference = request_reference)`,
				attemptID, receipt); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx,
				`UPDATE payments SET provider_payload = provider_payload || jsonb_build_object('receipt', $2::text)
				 WHERE id = $1 AND external_reference = $3
				 AND COALESCE(provider_payload->>'receipt', '') = ''`,
				paymentID, receipt, paymentReference); err != nil {
				return err
			}
			backfillExceptionReceipt = true
			return nil
		}
		if status != PaymentStatusPending && status != PaymentStatusFailed && status != PaymentStatusCaptured && status != PaymentStatusRefunded {
			return fmt.Errorf("%w: payment is %s", ErrPaymentNotFound, status)
		}
		var data map[string]string
		if err := json.Unmarshal(payload, &data); err != nil {
			return err
		}
		expected := money.Cents(amount)
		if charged, err := strconv.ParseInt(data["charged_cents"], 10, 64); err == nil && charged > 0 {
			expected = money.Cents(charged)
		}
		if confirmed != expected || !strings.EqualFold(cur, currency) {
			return ErrPaymentAmountMatch
		}
		var orderStatus string
		if err := tx.QueryRowContext(ctx, "SELECT status FROM orders WHERE id = $1 FOR UPDATE", orderID).Scan(&orderStatus); err != nil {
			return err
		}
		exceptionReason := ""
		if status == PaymentStatusCaptured || status == PaymentStatusRefunded {
			exceptionReason = "duplicate_charge"
		} else if status == PaymentStatusFailed || orderStatus == OrderStatusCancelled {
			exceptionReason = "late_capture"
		}
		if exceptionReason != "" {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO payment_exceptions
				 (payment_id, attempt_id, reason, provider, currency, amount_cents, request_reference, receipt)
				 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
				 ON CONFLICT (attempt_id) DO NOTHING`,
				paymentID, attemptID, exceptionReason, provider, cur, int64(confirmed), reference, receipt); err != nil {
				return err
			}
		}
		if status == PaymentStatusCaptured || status == PaymentStatusRefunded {
			_, err := tx.ExecContext(ctx,
				`UPDATE payment_attempts SET status = 'completed',
				 external_reference = COALESCE(NULLIF($2, ''), external_reference) WHERE id = $1`, attemptID, receipt)
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE payments SET status = 'captured', captured_at = now(),
			   provider_payload = provider_payload || jsonb_build_object('receipt', $2::text)
			 WHERE id = $1`, paymentID, receipt); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE payment_attempts SET status = 'completed', external_reference = COALESCE(NULLIF($2, ''), external_reference)
			 WHERE id = $1`, attemptID, receipt); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE payment_attempts SET status = 'failed', error_message = 'another attempt completed'
			 WHERE payment_id = $1 AND id <> $2 AND status = 'pending'`, paymentID, attemptID); err != nil {
			return err
		}
		captured = true
		return nil
	})
	if err != nil {
		return false, err
	}
	if backfillExceptionReceipt {
		if _, err := s.DB.ExecContext(ctx,
			`UPDATE payment_exceptions SET receipt = $2
			 WHERE attempt_id = $1 AND receipt = ''`, attemptID, receipt); err != nil {
			return false, err
		}
	}
	return captured, nil
}

// FailPaymentAttempt records a failed provider attempt. The payment stays
// pending so the customer can retry until the unpaid order expires.
func (s *Store) FailPaymentAttempt(ctx context.Context, attemptID int, reason string) error {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	_, err := s.DB.ExecContext(ctx,
		`UPDATE payment_attempts SET status = 'failed', error_message = $2
		 WHERE id = $1 AND status = 'pending'`, attemptID, truncateText(reason, 500))
	return err
}

// LastPaymentError returns the error message when the newest attempt failed,
// or "" when it succeeded, is still pending, or none exist.
func (s *Store) LastPaymentError(ctx context.Context, paymentID int) (string, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	var status string
	var msg sql.NullString
	err := s.DB.QueryRowContext(ctx,
		`SELECT status, error_message FROM payment_attempts WHERE payment_id = $1 ORDER BY id DESC LIMIT 1`, paymentID).Scan(&status, &msg)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if status != PaymentAttemptStatusFailed {
		return "", nil
	}
	return msg.String, nil
}

// ExpireUnpaidOrders cancels orders whose online payment is still pending
// after cutoff, releasing their stock and failing the payment.
func (s *Store) ExpireUnpaidOrders(ctx context.Context, cutoff time.Time) (int, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()

	// Discover candidates without retaining payment or inventory locks across
	// orders. Each cancellation commits independently, so a batch cannot hold
	// stock rows from one order while acquiring a lower-ID row for another.
	rows, err := s.DB.QueryContext(ctx,
		`SELECT p.id, p.order_id FROM payments p
		 WHERE p.status = 'pending' AND p.method <> 'cash_on_delivery' AND p.created_at < $1
		   AND NOT EXISTS (SELECT 1 FROM payment_attempts a WHERE a.payment_id = p.id
		                   AND a.status = 'pending' AND a.created_at >= $1)
		 ORDER BY p.order_id, p.id`, cutoff.UTC())
	if err != nil {
		return 0, err
	}
	type pair struct{ payment, order int }
	var stale []pair
	for rows.Next() {
		var p pair
		if err := rows.Scan(&p.payment, &p.order); err != nil {
			rows.Close()
			return 0, err
		}
		stale = append(stale, p)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}

	expired := 0
	for _, candidate := range stale {
		expiredThisPayment := false
		err := s.withTx(ctx, func(tx *sql.Tx) error {
			expiredThisPayment = false
			var paymentID int
			err := tx.QueryRowContext(ctx,
				`SELECT p.id FROM payments p
				 WHERE p.id = $1 AND p.status = 'pending' AND p.method <> 'cash_on_delivery' AND p.created_at < $2
				   AND NOT EXISTS (SELECT 1 FROM payment_attempts a WHERE a.payment_id = p.id
				                   AND a.status = 'pending' AND a.created_at >= $2)
				 FOR UPDATE OF p SKIP LOCKED`,
				candidate.payment, cutoff.UTC(),
			).Scan(&paymentID)
			if errors.Is(err, sql.ErrNoRows) {
				return nil // Another request changed it or a newer attempt is active.
			}
			if err != nil {
				return err
			}

			current, err := lockOrderTx(ctx, tx, candidate.order)
			if err != nil {
				return err
			}
			if err := writeOrderStateTx(ctx, tx, candidate.order, current, OrderStatusCancelled, "cancelled", current.DeliveryStatus,
				"Payment was not completed in time. The order was cancelled."); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, "UPDATE payments SET status = 'failed' WHERE id = $1", paymentID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx,
				`UPDATE payment_attempts SET status = 'failed', error_message = 'expired' WHERE payment_id = $1 AND status = 'pending'`,
				paymentID); err != nil {
				return err
			}
			expiredThisPayment = true
			return nil
		})
		if err != nil {
			return expired, err
		}
		if expiredThisPayment {
			expired++
		}
	}
	return expired, nil
}

// ErrOrderNotCancellable means the order has been accepted by the vendor or
// already paid, so only the marketplace team can cancel it.
var ErrOrderNotCancellable = errors.New("order can no longer be cancelled")

// CancelOrderByCustomer cancels a customer's own order while the vendor has
// not accepted it and nothing has been charged. Stock is released and any
// outstanding payment request is closed. A charge confirmed after this point
// is recorded as a late capture and queued for refund by CapturePayment.
func (s *Store) CancelOrderByCustomer(ctx context.Context, userID, orderID int) error {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	return s.withTx(ctx, func(tx *sql.Tx) error {
		// Lock payments before the order, the same order CapturePayment uses.
		rows, err := tx.QueryContext(ctx,
			"SELECT status FROM payments WHERE order_id = $1 AND user_id = $2 ORDER BY id FOR UPDATE", orderID, userID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var status string
			if err := rows.Scan(&status); err != nil {
				rows.Close()
				return err
			}
			if status == PaymentStatusCaptured || status == PaymentStatusRefunded {
				rows.Close()
				return ErrOrderNotCancellable
			}
		}
		if err := rows.Close(); err != nil {
			return err
		}
		var owner int
		if err := tx.QueryRowContext(ctx, "SELECT user_id FROM orders WHERE id = $1", orderID).Scan(&owner); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrOrderNotFound
			}
			return err
		}
		if owner != userID {
			return ErrOrderNotFound
		}
		current, err := lockOrderTx(ctx, tx, orderID)
		if err != nil {
			return err
		}
		if current.Status == OrderStatusCancelled {
			return nil
		}
		if current.Status != OrderStatusPending || current.PartnerStatus != "new" {
			return ErrOrderNotCancellable
		}
		if err := writeOrderStateTx(ctx, tx, orderID, current, OrderStatusCancelled, "cancelled", current.DeliveryStatus,
			"Cancelled by the customer."); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE payment_attempts SET status = 'failed', error_message = 'order cancelled'
			 WHERE status = 'pending' AND payment_id IN (SELECT id FROM payments WHERE order_id = $1)`, orderID); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "UPDATE payments SET status = 'failed' WHERE order_id = $1 AND status = 'pending'", orderID)
		return err
	})
}

func truncateText(s string, max int) string {
	if r := []rune(s); len(r) > max {
		return string(r[:max])
	}
	return s
}

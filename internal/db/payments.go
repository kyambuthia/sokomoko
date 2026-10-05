package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/money"
)

const checkoutCompleteOperation = "checkout.complete"

type PaymentRecordInput struct {
	Method            string
	Provider          string
	Status            string
	Currency          string
	Amount            money.Cents
	ExternalReference string
}

func checkoutPlacementTx(ctx context.Context, q querier, userID int, key string) (*CheckoutPlacement, error) {
	var placement CheckoutPlacement
	err := q.QueryRowContext(ctx,
		`SELECT ik.order_id, ik.payment_id, o.total_cents
		 FROM idempotency_keys ik
		 JOIN orders o ON o.id = ik.order_id
		 WHERE ik.user_id = $1 AND ik.operation = $2 AND ik.idempotency_key = $3 AND ik.completed_at IS NOT NULL`,
		userID, checkoutCompleteOperation, key,
	).Scan(&placement.OrderID, &placement.PaymentID, &placement.TotalAmount)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	placement.Reused = true
	return &placement, nil
}

// GetCheckoutPlacementByIdempotency returns a previously completed placement for
// the key, or nil.
func (s *Store) GetCheckoutPlacementByIdempotency(userID int, idempotencyKey string) (*CheckoutPlacement, error) {
	key := strings.TrimSpace(idempotencyKey)
	if key == "" {
		return nil, nil
	}
	ctx, cancel := s.ctx()
	defer cancel()
	return checkoutPlacementTx(ctx, s.DB, userID, key)
}

func (s *Store) ListPaymentsByOrderID(orderID int) ([]Payment, error) {
	ctx, cancel := s.ctx()
	defer cancel()

	rows, err := s.DB.QueryContext(ctx,
		`SELECT id, order_id, user_id, method, provider, status, currency, amount_cents, external_reference, created_at, updated_at
		 FROM payments
		 WHERE order_id = $1
		 ORDER BY id`,
		orderID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	payments := []Payment{}
	for rows.Next() {
		var p Payment
		if err := rows.Scan(&p.ID, &p.OrderID, &p.UserID, &p.Method, &p.Provider, &p.Status, &p.Currency, &p.Amount, &p.ExternalReference, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		payments = append(payments, p)
	}
	return payments, rows.Err()
}

// PlaceOrderFromCheckoutWithPayment converts an open checkout into an order,
// turning its stock reservations into allocations and recording the payment.
// The checkout token doubles as the idempotency key: repeating the call after
// success returns the original placement with Reused set.
func (s *Store) PlaceOrderFromCheckoutWithPayment(userID int, checkoutToken string, deliveryAddress string, deliveryNotice string, payment PaymentRecordInput) (CheckoutPlacement, error) {
	address := strings.TrimSpace(deliveryAddress)
	if address == "" {
		return CheckoutPlacement{}, ErrDeliveryAddressRequired
	}
	key := strings.TrimSpace(checkoutToken)
	if key == "" {
		return CheckoutPlacement{}, ErrCheckoutNotFound
	}
	ctx, cancel := s.ctx()
	defer cancel()

	var placement CheckoutPlacement
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		// Concurrent submissions of the same key serialize on this insert; the
		// loser sees the winner's completed placement below.
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO idempotency_keys (user_id, operation, idempotency_key)
			 VALUES ($1, $2, $3)
			 ON CONFLICT (user_id, operation, idempotency_key) DO NOTHING`,
			userID, checkoutCompleteOperation, key,
		); err != nil {
			return err
		}
		if existing, err := checkoutPlacementTx(ctx, tx, userID, key); err != nil {
			return err
		} else if existing != nil {
			placement = *existing
			return nil
		}

		checkout, err := scanCheckout(tx.QueryRowContext(ctx,
			"SELECT "+checkoutColumns+" FROM checkouts WHERE user_id = $1 AND token = $2 FOR UPDATE",
			userID, key,
		))
		if err != nil {
			return err
		}
		if checkout == nil {
			return ErrCheckoutNotFound
		}
		switch {
		case checkout.Status == CheckoutStatusExpired:
			return ErrCheckoutExpired
		case checkout.Status != CheckoutStatusOpen:
			return ErrCheckoutNotFound
		case !checkout.ExpiresAt.After(timeNow()):
			return ErrCheckoutExpired
		}

		lines, err := checkoutOrderLinesTx(ctx, tx, checkout.ID)
		if err != nil {
			return err
		}
		if len(lines) == 0 {
			return ErrCartEmpty
		}

		totals := OrderTotals{
			Subtotal:    checkout.SubtotalAmount,
			ShippingFee: checkout.ShippingFee,
			TaxAmount:   checkout.TaxAmount,
			Total:       checkout.TotalAmount,
		}
		orderID, err := createOrderTx(ctx, tx, userID, address, deliveryNotice, totals, lines,
			allocation{userID: userID, reservationKey: key})
		if err != nil {
			return err
		}

		payment.Method = strings.TrimSpace(payment.Method)
		payment.Amount = checkout.TotalAmount
		if strings.TrimSpace(payment.Currency) == "" {
			payment.Currency = checkout.Currency
		}
		paymentID, err := createPaymentTx(ctx, tx, orderID, userID, payment)
		if err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx,
			`UPDATE checkouts
			 SET status = 'completed', payment_method = COALESCE(NULLIF($1, ''), payment_method),
			     delivery_address = $2, order_id = $3, completed_at = now()
			 WHERE id = $4`,
			payment.Method, address, orderID, checkout.ID,
		); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE idempotency_keys
			 SET order_id = $1, payment_id = $2, completed_at = now()
			 WHERE user_id = $3 AND operation = $4 AND idempotency_key = $5`,
			orderID, paymentID, userID, checkoutCompleteOperation, key,
		); err != nil {
			return err
		}
		if err := clearCartTx(ctx, tx, userID); err != nil {
			return err
		}

		placement = CheckoutPlacement{OrderID: orderID, PaymentID: paymentID, TotalAmount: checkout.TotalAmount}
		return nil
	})
	return placement, err
}

func checkoutOrderLinesTx(ctx context.Context, tx *sql.Tx, checkoutID int) ([]orderLine, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT cl.product_id, p.partner_id, cl.product_name, cl.quantity, cl.unit_price_cents
		 FROM checkout_lines cl
		 JOIN products p ON p.id = cl.product_id
		 WHERE cl.checkout_id = $1
		 ORDER BY cl.product_id`,
		checkoutID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	lines := []orderLine{}
	for rows.Next() {
		var line orderLine
		if err := rows.Scan(&line.ProductID, &line.PartnerID, &line.ProductName, &line.Quantity, &line.UnitPrice); err != nil {
			return nil, err
		}
		lines = append(lines, line)
	}
	return lines, rows.Err()
}

func createPaymentTx(ctx context.Context, tx *sql.Tx, orderID int64, userID int, payment PaymentRecordInput) (int64, error) {
	var paymentID int64
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO payments (order_id, user_id, method, provider, status, currency, amount_cents, external_reference)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING id`,
		orderID,
		userID,
		strings.TrimSpace(payment.Method),
		strings.TrimSpace(payment.Provider),
		strings.TrimSpace(payment.Status),
		strings.TrimSpace(payment.Currency),
		int64(payment.Amount),
		nullIfEmpty(payment.ExternalReference),
	).Scan(&paymentID); err != nil {
		return 0, wrapDBError(err)
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO payment_attempts (payment_id, status, request_reference, external_reference)
		 VALUES ($1, $2, $3, $4)`,
		paymentID,
		PaymentAttemptStatusCompleted,
		fmt.Sprintf("payment:%d", paymentID),
		nullIfEmpty(payment.ExternalReference),
	); err != nil {
		return 0, err
	}
	return paymentID, nil
}

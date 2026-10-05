package db

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

const checkoutColumns = `id, user_id, token, status, currency, payment_method,
       subtotal_cents, shipping_cents, tax_cents, total_cents,
       delivery_address, expires_at, completed_at, order_id, created_at, updated_at`

func scanCheckout(row interface{ Scan(...any) error }) (*Checkout, error) {
	checkout := &Checkout{}
	err := row.Scan(
		&checkout.ID, &checkout.UserID, &checkout.Token, &checkout.Status, &checkout.Currency,
		&checkout.PaymentMethod, &checkout.SubtotalAmount, &checkout.ShippingFee, &checkout.TaxAmount,
		&checkout.TotalAmount, &checkout.DeliveryAddress, &checkout.ExpiresAt, &checkout.CompletedAt,
		&checkout.OrderID, &checkout.CreatedAt, &checkout.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return checkout, nil
}

func checkoutLines(ctx context.Context, q querier, checkoutID int) ([]CheckoutLine, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id, checkout_id, product_id, product_name, quantity, unit_price_cents, line_total_cents
		 FROM checkout_lines
		 WHERE checkout_id = $1
		 ORDER BY product_id`,
		checkoutID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	lines := []CheckoutLine{}
	for rows.Next() {
		var line CheckoutLine
		if err := rows.Scan(&line.ID, &line.CheckoutID, &line.ProductID, &line.ProductName, &line.Quantity, &line.UnitPrice, &line.LineTotal); err != nil {
			return nil, err
		}
		lines = append(lines, line)
	}
	return lines, rows.Err()
}

func (s *Store) loadCheckout(ctx context.Context, q querier, where string, args ...any) (*Checkout, error) {
	checkout, err := scanCheckout(q.QueryRowContext(ctx, "SELECT "+checkoutColumns+" FROM checkouts "+where, args...))
	if err != nil || checkout == nil {
		return checkout, err
	}
	checkout.Lines, err = checkoutLines(ctx, q, checkout.ID)
	if err != nil {
		return nil, err
	}
	return checkout, nil
}

func (s *Store) GetCheckoutByToken(userID int, token string) (*Checkout, error) {
	key := strings.TrimSpace(token)
	if key == "" {
		return nil, nil
	}
	ctx, cancel := s.ctx()
	defer cancel()
	return s.loadCheckout(ctx, s.DB, "WHERE user_id = $1 AND token = $2", userID, key)
}

// GetLatestOpenCheckout returns the user's most recent unexpired open checkout.
func (s *Store) GetLatestOpenCheckout(userID int) (*Checkout, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	return s.loadCheckout(ctx, s.DB,
		`WHERE user_id = $1 AND status = 'open' AND expires_at > now()
		 ORDER BY updated_at DESC, id DESC LIMIT 1`,
		userID,
	)
}

// UpsertCheckout creates or refreshes the open checkout identified by token,
// replacing its lines. Other open checkouts for the user are cancelled.
func (s *Store) UpsertCheckout(userID int, input CheckoutInput) (*Checkout, error) {
	token := strings.TrimSpace(input.Token)
	if token == "" {
		return nil, errors.New("checkout token is required")
	}
	if len(input.Lines) == 0 {
		return nil, ErrCartEmpty
	}
	paymentMethod := strings.TrimSpace(input.PaymentMethod)
	if paymentMethod == "" {
		paymentMethod = "cash_on_delivery"
	}
	currency := strings.TrimSpace(input.Currency)
	if currency == "" {
		currency = "USD"
	}
	expiresAt := input.ExpiresAt.UTC()
	if expiresAt.IsZero() {
		expiresAt = time.Now().UTC()
	}

	ctx, cancel := s.ctx()
	defer cancel()

	var checkout *Checkout
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			"UPDATE checkouts SET status = 'cancelled' WHERE user_id = $1 AND status = 'open' AND token <> $2",
			userID, token,
		); err != nil {
			return err
		}

		var checkoutID int
		err := tx.QueryRowContext(ctx,
			`INSERT INTO checkouts (user_id, token, status, currency, payment_method,
			                        subtotal_cents, shipping_cents, tax_cents, total_cents, expires_at)
			 VALUES ($1, $2, 'open', $3, $4, $5, $6, $7, $8, $9)
			 ON CONFLICT (token) DO UPDATE SET
			     status = 'open',
			     currency = EXCLUDED.currency,
			     payment_method = EXCLUDED.payment_method,
			     subtotal_cents = EXCLUDED.subtotal_cents,
			     shipping_cents = EXCLUDED.shipping_cents,
			     tax_cents = EXCLUDED.tax_cents,
			     total_cents = EXCLUDED.total_cents,
			     expires_at = EXCLUDED.expires_at,
			     completed_at = NULL,
			     order_id = NULL
			 WHERE checkouts.user_id = EXCLUDED.user_id AND checkouts.status <> 'completed'
			 RETURNING id`,
			userID, token, currency, paymentMethod,
			int64(input.SubtotalAmount), int64(input.ShippingFee), int64(input.TaxAmount), int64(input.TotalAmount),
			expiresAt,
		).Scan(&checkoutID)
		if errors.Is(err, sql.ErrNoRows) {
			// The token belongs to another user or an already completed checkout.
			return ErrCheckoutNotFound
		}
		if err != nil {
			return wrapDBError(err)
		}

		if _, err := tx.ExecContext(ctx, "DELETE FROM checkout_lines WHERE checkout_id = $1", checkoutID); err != nil {
			return err
		}
		for _, line := range input.Lines {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO checkout_lines (checkout_id, product_id, product_name, quantity, unit_price_cents)
				 VALUES ($1, $2, $3, $4, $5)`,
				checkoutID, line.ProductID, strings.TrimSpace(line.ProductName), line.Quantity, int64(line.UnitPrice),
			); err != nil {
				return err
			}
		}

		checkout, err = s.loadCheckout(ctx, tx, "WHERE id = $1", checkoutID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return checkout, nil
}

// UpdateCheckoutDraft saves the delivery address and payment method entered on
// an open checkout. An empty payment method keeps the current one.
func (s *Store) UpdateCheckoutDraft(userID int, token string, deliveryAddress string, paymentMethod string) (*Checkout, error) {
	key := strings.TrimSpace(token)
	if key == "" {
		return nil, ErrCheckoutNotFound
	}
	ctx, cancel := s.ctx()
	defer cancel()

	var checkout *Checkout
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx,
			`UPDATE checkouts
			 SET delivery_address = $1,
			     payment_method = COALESCE(NULLIF($2, ''), payment_method)
			 WHERE user_id = $3 AND token = $4 AND status = 'open' AND expires_at > now()`,
			strings.TrimSpace(deliveryAddress), strings.TrimSpace(paymentMethod), userID, key,
		)
		if err != nil {
			return err
		}
		affected, err := rowsAffected(result)
		if err != nil {
			return err
		}
		if affected == 0 {
			return ErrCheckoutNotFound
		}
		checkout, err = s.loadCheckout(ctx, tx, "WHERE user_id = $1 AND token = $2", userID, key)
		return err
	})
	if err != nil {
		return nil, err
	}
	return checkout, nil
}

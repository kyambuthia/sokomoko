package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

func (s *Store) GetCheckoutByToken(userID int, token string) (*Checkout, error) {
	key := strings.TrimSpace(token)
	if key == "" {
		return nil, nil
	}

	checkout, err := getCheckoutByTokenQuerier(s.DB, userID, key)
	if err != nil {
		return nil, err
	}
	if checkout == nil {
		return nil, nil
	}

	lines, err := listCheckoutLinesByCheckoutIDQuerier(s.DB, checkout.ID)
	if err != nil {
		return nil, err
	}
	checkout.Lines = lines
	return checkout, nil
}

func (s *Store) UpsertCheckout(userID int, input CheckoutInput) (*Checkout, error) {
	token := strings.TrimSpace(input.Token)
	if token == "" {
		return nil, fmt.Errorf("checkout token is required")
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
	now := time.Now().UTC()
	if expiresAt.IsZero() {
		expiresAt = now
	}

	tx, err := s.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if err := expireOpenCheckoutsTx(tx, now); err != nil {
		return nil, err
	}
	if err := cancelOtherOpenCheckoutsForUserTx(tx, userID, token); err != nil {
		return nil, err
	}

	if _, err := tx.Exec(
		`INSERT INTO checkouts (
		     user_id, token, status, currency, payment_method,
		     subtotal_amount, shipping_fee, tax_amount, total_amount, expires_at
		 )
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(token) DO UPDATE SET
		     user_id = excluded.user_id,
		     status = excluded.status,
		     currency = excluded.currency,
		     payment_method = excluded.payment_method,
		     subtotal_amount = excluded.subtotal_amount,
		     shipping_fee = excluded.shipping_fee,
		     tax_amount = excluded.tax_amount,
		     total_amount = excluded.total_amount,
		     expires_at = excluded.expires_at,
		     completed_at = NULL,
		     order_id = NULL,
		     updated_at = CURRENT_TIMESTAMP`,
		userID,
		token,
		CheckoutStatusOpen,
		currency,
		paymentMethod,
		input.SubtotalAmount,
		input.ShippingFee,
		input.TaxAmount,
		input.TotalAmount,
		expiresAt,
	); err != nil {
		return nil, err
	}

	checkout, err := getCheckoutByTokenQuerier(tx, userID, token)
	if err != nil {
		return nil, err
	}
	if checkout == nil {
		return nil, ErrCheckoutNotFound
	}

	if _, err := tx.Exec("DELETE FROM checkout_lines WHERE checkout_id = ?", checkout.ID); err != nil {
		return nil, err
	}

	for _, line := range input.Lines {
		if _, err := tx.Exec(
			`INSERT INTO checkout_lines (
			     checkout_id, product_id, product_name, quantity, unit_price, line_total, reservation_key
			 )
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			checkout.ID,
			line.ProductID,
			strings.TrimSpace(line.ProductName),
			line.Quantity,
			line.UnitPrice,
			line.LineTotal,
			nullIfEmpty(line.ReservationKey),
		); err != nil {
			return nil, err
		}
	}

	lines, err := listCheckoutLinesByCheckoutIDQuerier(tx, checkout.ID)
	if err != nil {
		return nil, err
	}
	checkout.Lines = lines

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return checkout, nil
}

type checkoutQuerier interface {
	QueryRow(query string, args ...any) *sql.Row
	Query(query string, args ...any) (*sql.Rows, error)
}

func getCheckoutByTokenQuerier(q checkoutQuerier, userID int, token string) (*Checkout, error) {
	checkout := &Checkout{}
	err := q.QueryRow(
		`SELECT id, user_id, token, status, currency, payment_method,
		        subtotal_amount, shipping_fee, tax_amount, total_amount,
		        delivery_address, expires_at, completed_at, order_id, created_at, updated_at
		 FROM checkouts
		 WHERE user_id = ? AND token = ?`,
		userID,
		token,
	).Scan(
		&checkout.ID,
		&checkout.UserID,
		&checkout.Token,
		&checkout.Status,
		&checkout.Currency,
		&checkout.PaymentMethod,
		&checkout.SubtotalAmount,
		&checkout.ShippingFee,
		&checkout.TaxAmount,
		&checkout.TotalAmount,
		&checkout.DeliveryAddress,
		&checkout.ExpiresAt,
		&checkout.CompletedAt,
		&checkout.OrderID,
		&checkout.CreatedAt,
		&checkout.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return checkout, nil
}

func listCheckoutLinesByCheckoutIDQuerier(q checkoutQuerier, checkoutID int) ([]CheckoutLine, error) {
	rows, err := q.Query(
		`SELECT id, checkout_id, product_id, product_name, quantity, unit_price, line_total, reservation_key, created_at, updated_at
		 FROM checkout_lines
		 WHERE checkout_id = ?
		 ORDER BY id`,
		checkoutID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	lines := []CheckoutLine{}
	for rows.Next() {
		line := CheckoutLine{}
		if err := rows.Scan(
			&line.ID,
			&line.CheckoutID,
			&line.ProductID,
			&line.ProductName,
			&line.Quantity,
			&line.UnitPrice,
			&line.LineTotal,
			&line.ReservationKey,
			&line.CreatedAt,
			&line.UpdatedAt,
		); err != nil {
			return nil, err
		}
		lines = append(lines, line)
	}
	return lines, rows.Err()
}

func expireOpenCheckoutsTx(tx *sql.Tx, now time.Time) error {
	_, err := tx.Exec(
		`UPDATE checkouts
		 SET status = ?, updated_at = CURRENT_TIMESTAMP
		 WHERE status = ? AND expires_at IS NOT NULL AND expires_at <= ?`,
		CheckoutStatusExpired,
		CheckoutStatusOpen,
		now.UTC(),
	)
	return err
}

func cancelOtherOpenCheckoutsForUserTx(tx *sql.Tx, userID int, token string) error {
	_, err := tx.Exec(
		`UPDATE checkouts
		 SET status = ?, updated_at = CURRENT_TIMESTAMP
		 WHERE user_id = ? AND status = ? AND token <> ?`,
		CheckoutStatusCancelled,
		userID,
		CheckoutStatusOpen,
		token,
	)
	return err
}

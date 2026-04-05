package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const checkoutCompleteOperation = "checkout.complete"

type PaymentRecordInput struct {
	Method            string
	Provider          string
	Status            string
	Currency          string
	Amount            float64
	ExternalReference string
}

func (s *Store) GetCheckoutPlacementByIdempotency(userID int, idempotencyKey string) (*CheckoutPlacement, error) {
	key := strings.TrimSpace(idempotencyKey)
	if key == "" {
		return nil, nil
	}

	row := s.DB.QueryRow(
		`SELECT ik.order_id, ik.payment_id, o.total_amount
		 FROM idempotency_keys ik
		 JOIN orders o ON o.id = ik.order_id
		 WHERE ik.user_id = ? AND ik.operation = ? AND ik.idempotency_key = ? AND ik.completed_at IS NOT NULL`,
		userID, checkoutCompleteOperation, key,
	)

	var placement CheckoutPlacement
	err := row.Scan(&placement.OrderID, &placement.PaymentID, &placement.TotalAmount)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	placement.Reused = true
	return &placement, nil
}

func (s *Store) ListPaymentsByOrderID(orderID int) ([]Payment, error) {
	rows, err := s.DB.Query(
		`SELECT id, order_id, user_id, method, provider, status, currency, amount, external_reference, created_at, updated_at
		 FROM payments
		 WHERE order_id = ?
		 ORDER BY id`,
		orderID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	payments := []Payment{}
	for rows.Next() {
		payment := Payment{}
		if err := rows.Scan(
			&payment.ID,
			&payment.OrderID,
			&payment.UserID,
			&payment.Method,
			&payment.Provider,
			&payment.Status,
			&payment.Currency,
			&payment.Amount,
			&payment.ExternalReference,
			&payment.CreatedAt,
			&payment.UpdatedAt,
		); err != nil {
			return nil, err
		}
		payments = append(payments, payment)
	}
	return payments, rows.Err()
}

func (s *Store) PlaceOrderFromCartWithPricingAndPayment(userID int, deliveryAddress string, totalAmount float64, deliveryNotice string, payment PaymentRecordInput, idempotencyKey string) (CheckoutPlacement, error) {
	address := strings.TrimSpace(deliveryAddress)
	if address == "" {
		return CheckoutPlacement{}, ErrDeliveryAddressRequired
	}
	if totalAmount < 0 {
		return CheckoutPlacement{}, ErrNegativeTotalAmount
	}

	tx, err := s.DB.Begin()
	if err != nil {
		return CheckoutPlacement{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if err = expireActiveStockReservationsTx(tx, time.Now().UTC()); err != nil {
		return CheckoutPlacement{}, err
	}

	key := strings.TrimSpace(idempotencyKey)
	if key != "" {
		placement, placementErr := getCheckoutPlacementTx(tx, userID, key)
		if placementErr != nil {
			return CheckoutPlacement{}, placementErr
		}
		if placement != nil {
			return *placement, nil
		}

		if _, err = tx.Exec(
			`INSERT INTO idempotency_keys (user_id, operation, idempotency_key)
			 VALUES (?, ?, ?)`,
			userID, checkoutCompleteOperation, key,
		); err != nil {
			placement, placementErr = getCheckoutPlacementTx(tx, userID, key)
			if placementErr != nil {
				return CheckoutPlacement{}, placementErr
			}
			if placement != nil {
				return *placement, nil
			}
			return CheckoutPlacement{}, err
		}
	}

	orderID, orderErr := placeOrderFromCartTx(tx, userID, address, totalAmount, strings.TrimSpace(deliveryNotice), true, key)
	if orderErr != nil {
		return CheckoutPlacement{}, orderErr
	}

	paymentID, paymentErr := createPaymentTx(tx, orderID, userID, payment)
	if paymentErr != nil {
		return CheckoutPlacement{}, paymentErr
	}

	if key != "" {
		if _, err = tx.Exec(
			`UPDATE idempotency_keys
			 SET order_id = ?, payment_id = ?, completed_at = CURRENT_TIMESTAMP
			 WHERE user_id = ? AND operation = ? AND idempotency_key = ?`,
			orderID, paymentID, userID, checkoutCompleteOperation, key,
		); err != nil {
			return CheckoutPlacement{}, err
		}
	}

	if err = tx.Commit(); err != nil {
		return CheckoutPlacement{}, err
	}

	return CheckoutPlacement{
		OrderID:     orderID,
		PaymentID:   paymentID,
		TotalAmount: payment.Amount,
	}, nil
}

func (s *Store) PlaceOrderFromCheckoutWithPayment(userID int, checkoutToken string, deliveryAddress string, deliveryNotice string, payment PaymentRecordInput) (CheckoutPlacement, error) {
	address := strings.TrimSpace(deliveryAddress)
	if address == "" {
		return CheckoutPlacement{}, ErrDeliveryAddressRequired
	}

	key := strings.TrimSpace(checkoutToken)
	if key == "" {
		return CheckoutPlacement{}, ErrCheckoutNotFound
	}

	tx, err := s.DB.Begin()
	if err != nil {
		return CheckoutPlacement{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	now := time.Now().UTC()
	if err = expireActiveStockReservationsTx(tx, now); err != nil {
		return CheckoutPlacement{}, err
	}
	if err = expireOpenCheckoutsTx(tx, now); err != nil {
		return CheckoutPlacement{}, err
	}

	if placement, placementErr := getCheckoutPlacementTx(tx, userID, key); placementErr != nil {
		return CheckoutPlacement{}, placementErr
	} else if placement != nil {
		return *placement, nil
	}

	if _, err = tx.Exec(
		`INSERT INTO idempotency_keys (user_id, operation, idempotency_key)
		 VALUES (?, ?, ?)`,
		userID, checkoutCompleteOperation, key,
	); err != nil {
		placement, placementErr := getCheckoutPlacementTx(tx, userID, key)
		if placementErr != nil {
			return CheckoutPlacement{}, placementErr
		}
		if placement != nil {
			return *placement, nil
		}
		return CheckoutPlacement{}, err
	}

	checkout, err := getCheckoutByTokenQuerier(tx, userID, key)
	if err != nil {
		return CheckoutPlacement{}, err
	}
	if checkout == nil {
		return CheckoutPlacement{}, ErrCheckoutNotFound
	}
	if checkout.Status != CheckoutStatusOpen {
		if checkout.Status == CheckoutStatusExpired {
			return CheckoutPlacement{}, ErrCheckoutExpired
		}
		return CheckoutPlacement{}, ErrCheckoutNotFound
	}
	if checkout.ExpiresAt.Valid && !checkout.ExpiresAt.Time.After(now) {
		if _, err := tx.Exec(
			`UPDATE checkouts
			 SET status = ?, updated_at = CURRENT_TIMESTAMP
			 WHERE id = ?`,
			CheckoutStatusExpired,
			checkout.ID,
		); err != nil {
			return CheckoutPlacement{}, err
		}
		return CheckoutPlacement{}, ErrCheckoutExpired
	}

	lines, err := listCheckoutLinesByCheckoutIDQuerier(tx, checkout.ID)
	if err != nil {
		return CheckoutPlacement{}, err
	}
	if len(lines) == 0 {
		return CheckoutPlacement{}, ErrCartEmpty
	}

	orderLines := make([]orderPlacementLine, 0, len(lines))
	for _, line := range lines {
		orderLines = append(orderLines, orderPlacementLine{
			ProductID:     line.ProductID,
			ProductName:   line.ProductName,
			Quantity:      line.Quantity,
			UnitPrice:     line.UnitPrice,
			StockQuantity: line.Quantity,
		})
	}

	notice := strings.TrimSpace(deliveryNotice)
	if notice == "" {
		notice = "Order received. Awaiting partner acceptance."
	}

	payment.Method = strings.TrimSpace(payment.Method)
	payment.Amount = checkout.TotalAmount
	if strings.TrimSpace(payment.Currency) == "" {
		payment.Currency = checkout.Currency
	}

	orderID, orderErr := createOrderWithLinesTx(tx, userID, address, checkout.TotalAmount, notice, key, orderLines)
	if orderErr != nil {
		return CheckoutPlacement{}, orderErr
	}

	paymentID, paymentErr := createPaymentTx(tx, orderID, userID, payment)
	if paymentErr != nil {
		return CheckoutPlacement{}, paymentErr
	}

	if _, err = tx.Exec(
		`UPDATE checkouts
		 SET status = ?, payment_method = ?, delivery_address = ?, order_id = ?, completed_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		 WHERE id = ?`,
		CheckoutStatusCompleted,
		payment.Method,
		address,
		orderID,
		checkout.ID,
	); err != nil {
		return CheckoutPlacement{}, err
	}

	if _, err = tx.Exec(
		`UPDATE idempotency_keys
		 SET order_id = ?, payment_id = ?, completed_at = CURRENT_TIMESTAMP
		 WHERE user_id = ? AND operation = ? AND idempotency_key = ?`,
		orderID, paymentID, userID, checkoutCompleteOperation, key,
	); err != nil {
		return CheckoutPlacement{}, err
	}

	var cartID int64
	cartErr := tx.QueryRow("SELECT id FROM carts WHERE user_id = ?", userID).Scan(&cartID)
	if cartErr != nil && cartErr != sql.ErrNoRows {
		return CheckoutPlacement{}, cartErr
	}
	if cartErr == nil {
		if _, err = tx.Exec("DELETE FROM cart_items WHERE cart_id = ?", cartID); err != nil {
			return CheckoutPlacement{}, err
		}
	}

	if err = tx.Commit(); err != nil {
		return CheckoutPlacement{}, err
	}

	return CheckoutPlacement{
		OrderID:     orderID,
		PaymentID:   paymentID,
		TotalAmount: payment.Amount,
	}, nil
}

func getCheckoutPlacementTx(tx *sql.Tx, userID int, idempotencyKey string) (*CheckoutPlacement, error) {
	row := tx.QueryRow(
		`SELECT ik.order_id, ik.payment_id, o.total_amount
		 FROM idempotency_keys ik
		 JOIN orders o ON o.id = ik.order_id
		 WHERE ik.user_id = ? AND ik.operation = ? AND ik.idempotency_key = ? AND ik.completed_at IS NOT NULL`,
		userID, checkoutCompleteOperation, idempotencyKey,
	)

	var placement CheckoutPlacement
	err := row.Scan(&placement.OrderID, &placement.PaymentID, &placement.TotalAmount)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	placement.Reused = true
	return &placement, nil
}

func createPaymentTx(tx *sql.Tx, orderID int64, userID int, payment PaymentRecordInput) (int64, error) {
	paymentStmt, err := tx.Prepare(
		`INSERT INTO payments (order_id, user_id, method, provider, status, currency, amount, external_reference)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
	)
	if err != nil {
		return 0, err
	}
	defer paymentStmt.Close()

	res, err := paymentStmt.Exec(
		orderID,
		userID,
		strings.TrimSpace(payment.Method),
		strings.TrimSpace(payment.Provider),
		strings.TrimSpace(payment.Status),
		strings.TrimSpace(payment.Currency),
		payment.Amount,
		nullIfEmpty(payment.ExternalReference),
	)
	if err != nil {
		return 0, err
	}
	paymentID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	attemptStmt, err := tx.Prepare(
		`INSERT INTO payment_attempts (payment_id, status, request_reference, external_reference, error_message)
		 VALUES (?, ?, ?, ?, ?)`,
	)
	if err != nil {
		return 0, err
	}
	defer attemptStmt.Close()

	requestReference := fmt.Sprintf("payment:%d", paymentID)
	if _, err := attemptStmt.Exec(
		paymentID,
		PaymentAttemptStatusCompleted,
		requestReference,
		nullIfEmpty(payment.ExternalReference),
		nil,
	); err != nil {
		return 0, err
	}

	return paymentID, nil
}

func nullIfEmpty(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

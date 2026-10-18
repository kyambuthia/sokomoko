package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kyambuthia/sokomoko/internal/money"
)

var (
	ErrPaymentNotFound    = errors.New("payment not found")
	ErrCurrencyLocked     = errors.New("currency cannot change after orders exist")
	ErrPaymentAmountMatch = errors.New("payment amount or currency does not match")
)

// CommerceSettings are the marketplace-wide money rules.
type CommerceSettings struct {
	Currency              string
	ShippingFlat          money.Cents
	FreeShippingThreshold money.Cents // 0 disables free shipping
	TaxRateBP             int64       // basis points, 800 = 8%
}

// GetCommerceSettings returns the money settings. The migration seeds the row.
func (s *Store) GetCommerceSettings() (CommerceSettings, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	var c CommerceSettings
	var flat, free int64
	err := s.DB.QueryRowContext(ctx,
		`SELECT currency, shipping_flat_cents, free_shipping_threshold_cents, tax_rate_bp FROM commerce_settings WHERE id = 1`,
	).Scan(&c.Currency, &flat, &free, &c.TaxRateBP)
	c.ShippingFlat, c.FreeShippingThreshold = money.Cents(flat), money.Cents(free)
	return c, err
}

// UpdateCommerceSettings saves the money settings. The currency may only
// change while no orders exist, because stored prices are minor units of the
// old currency; product currencies follow the change.
func (s *Store) UpdateCommerceSettings(c CommerceSettings) error {
	ctx, cancel := s.ctx()
	defer cancel()
	return s.withTx(ctx, func(tx *sql.Tx) error {
		var current string
		if err := tx.QueryRowContext(ctx, "SELECT currency FROM commerce_settings WHERE id = 1 FOR UPDATE").Scan(&current); err != nil {
			return err
		}
		if current != c.Currency {
			var orders int
			if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM orders").Scan(&orders); err != nil {
				return err
			}
			if orders > 0 {
				return ErrCurrencyLocked
			}
			if _, err := tx.ExecContext(ctx, "UPDATE products SET currency = $1", c.Currency); err != nil {
				return err
			}
		}
		_, err := tx.ExecContext(ctx,
			`UPDATE commerce_settings
			 SET currency = $1, shipping_flat_cents = $2, free_shipping_threshold_cents = $3, tax_rate_bp = $4
			 WHERE id = 1`,
			c.Currency, int64(c.ShippingFlat), int64(c.FreeShippingThreshold), c.TaxRateBP,
		)
		return wrapDBError(err)
	})
}

// PaymentProviderConfig is one provider's stored configuration. Secrets stay
// sealed; only internal/payments opens them.
type PaymentProviderConfig struct {
	Provider  string
	Enabled   bool
	Mode      string // "test" or "live"
	Settings  map[string]string
	Secrets   []byte
	UpdatedAt time.Time
}

func scanProviderConfig(row interface{ Scan(...any) error }) (*PaymentProviderConfig, error) {
	var c PaymentProviderConfig
	var settings []byte
	if err := row.Scan(&c.Provider, &c.Enabled, &c.Mode, &settings, &c.Secrets, &c.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	c.Settings = map[string]string{}
	if len(settings) > 0 {
		if err := json.Unmarshal(settings, &c.Settings); err != nil {
			return nil, fmt.Errorf("decode %s settings: %w", c.Provider, err)
		}
	}
	return &c, nil
}

const providerColumns = "provider, enabled, mode, settings, secrets, updated_at"

func (s *Store) ListPaymentProviderConfigs() ([]PaymentProviderConfig, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	rows, err := s.DB.QueryContext(ctx, "SELECT "+providerColumns+" FROM payment_providers ORDER BY provider")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	configs := []PaymentProviderConfig{}
	for rows.Next() {
		c, err := scanProviderConfig(rows)
		if err != nil {
			return nil, err
		}
		configs = append(configs, *c)
	}
	return configs, rows.Err()
}

func (s *Store) GetPaymentProviderConfig(provider string) (*PaymentProviderConfig, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	return scanProviderConfig(s.DB.QueryRowContext(ctx, "SELECT "+providerColumns+" FROM payment_providers WHERE provider = $1", provider))
}

// SavePaymentProviderConfig upserts a provider. A nil Secrets keeps the stored
// secrets, so an admin can change settings without re-entering keys.
func (s *Store) SavePaymentProviderConfig(c PaymentProviderConfig) error {
	settings, err := json.Marshal(c.Settings)
	if err != nil {
		return err
	}
	ctx, cancel := s.ctx()
	defer cancel()
	_, err = s.DB.ExecContext(ctx,
		`INSERT INTO payment_providers (provider, enabled, mode, settings, secrets)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (provider) DO UPDATE SET
		   enabled = EXCLUDED.enabled,
		   mode = EXCLUDED.mode,
		   settings = EXCLUDED.settings,
		   secrets = COALESCE(EXCLUDED.secrets, payment_providers.secrets)`,
		c.Provider, c.Enabled, c.Mode, settings, c.Secrets,
	)
	return wrapDBError(err)
}

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
}

const onlinePaymentSelect = `
SELECT p.id, p.order_id, p.user_id, u.email, p.method, p.provider, p.status, p.currency, p.amount_cents,
       COALESCE(p.external_reference, ''), COALESCE(p.callback_token_hash, ''), o.status, p.created_at
FROM payments p
JOIN users u ON u.id = p.user_id
JOIN orders o ON o.id = p.order_id`

func scanOnlinePayment(row interface{ Scan(...any) error }) (*OnlinePayment, error) {
	var p OnlinePayment
	var amount int64
	err := row.Scan(&p.ID, &p.OrderID, &p.UserID, &p.UserEmail, &p.Method, &p.Provider, &p.Status, &p.Currency, &amount,
		&p.ExternalReference, &p.CallbackTokenHash, &p.OrderStatus, &p.CreatedAt)
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
func (s *Store) GetOrderPayment(userID, orderID int) (*OnlinePayment, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	return scanOnlinePayment(s.DB.QueryRowContext(ctx,
		onlinePaymentSelect+" WHERE p.order_id = $1 AND p.user_id = $2 ORDER BY p.id DESC LIMIT 1", orderID, userID))
}

func (s *Store) GetPaymentByID(paymentID int) (*OnlinePayment, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	return scanOnlinePayment(s.DB.QueryRowContext(ctx, onlinePaymentSelect+" WHERE p.id = $1", paymentID))
}

func (s *Store) GetPaymentByReference(provider, reference string) (*OnlinePayment, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	return scanOnlinePayment(s.DB.QueryRowContext(ctx,
		onlinePaymentSelect+" WHERE p.provider = $1 AND p.external_reference = $2", provider, reference))
}

// StartPaymentAttempt records that a provider request is being made for a
// pending payment: the new external reference, the hashed callback token,
// and the provider payload (for example M-Pesa request IDs).
func (s *Store) StartPaymentAttempt(paymentID int, reference, callbackTokenHash string, payload map[string]string) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	ctx, cancel := s.ctx()
	defer cancel()
	return s.withTx(ctx, func(tx *sql.Tx) error {
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
		_, err = tx.ExecContext(ctx,
			`INSERT INTO payment_attempts (payment_id, status, request_reference, external_reference)
			 VALUES ($1, 'pending', $2, $2)`, paymentID, reference)
		return err
	})
}

// UpdatePaymentPayload merges provider identifiers into the payment, for
// example the M-Pesa CheckoutRequestID returned after an STK push.
func (s *Store) UpdatePaymentPayload(paymentID int, payload map[string]string) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	ctx, cancel := s.ctx()
	defer cancel()
	_, err = s.DB.ExecContext(ctx, "UPDATE payments SET provider_payload = provider_payload || $1::jsonb WHERE id = $2", raw, paymentID)
	return err
}

// PaymentPayload returns the stored provider identifiers.
func (s *Store) PaymentPayload(paymentID int) (map[string]string, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	var raw []byte
	if err := s.DB.QueryRowContext(ctx, "SELECT provider_payload FROM payments WHERE id = $1", paymentID).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrPaymentNotFound
		}
		return nil, err
	}
	payload := map[string]string{}
	return payload, json.Unmarshal(raw, &payload)
}

// CapturePayment marks a pending payment captured after the provider
// confirmed it. The confirmed amount (minor units) and currency must match
// what was charged. Capturing an already captured payment is a no-op, so
// webhooks and return redirects can race safely. It reports whether this call
// performed the capture.
func (s *Store) CapturePayment(paymentID int, confirmed money.Cents, currency, receipt string) (bool, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	captured := false
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		var status, cur string
		var amount int64
		if err := tx.QueryRowContext(ctx, "SELECT status, currency, amount_cents FROM payments WHERE id = $1 FOR UPDATE", paymentID).
			Scan(&status, &cur, &amount); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrPaymentNotFound
			}
			return err
		}
		if status == PaymentStatusCaptured {
			return nil
		}
		if status != PaymentStatusPending {
			return fmt.Errorf("%w: payment is %s", ErrPaymentNotFound, status)
		}
		if confirmed < money.Cents(amount) || !strings.EqualFold(cur, currency) {
			return ErrPaymentAmountMatch
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE payments SET status = 'captured', captured_at = now(),
			   provider_payload = provider_payload || jsonb_build_object('receipt', $2::text)
			 WHERE id = $1`, paymentID, receipt); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE payment_attempts SET status = 'completed', external_reference = COALESCE(NULLIF($2, ''), external_reference)
			 WHERE payment_id = $1 AND status = 'pending'`, paymentID, receipt); err != nil {
			return err
		}
		captured = true
		return nil
	})
	return captured, err
}

// FailPaymentAttempt records a failed provider attempt. The payment stays
// pending so the customer can retry until the unpaid order expires.
func (s *Store) FailPaymentAttempt(paymentID int, reason string) error {
	ctx, cancel := s.ctx()
	defer cancel()
	_, err := s.DB.ExecContext(ctx,
		`UPDATE payment_attempts SET status = 'failed', error_message = $2
		 WHERE payment_id = $1 AND status = 'pending'`, paymentID, truncateText(reason, 500))
	return err
}

// LastPaymentError returns the error message when the newest attempt failed,
// or "" when it succeeded, is still pending, or none exist.
func (s *Store) LastPaymentError(paymentID int) (string, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	var status string
	var msg sql.NullString
	err := s.DB.QueryRowContext(ctx,
		`SELECT status, error_message FROM payment_attempts WHERE payment_id = $1 ORDER BY id DESC LIMIT 1`, paymentID).Scan(&status, &msg)
	if errors.Is(err, sql.ErrNoRows) || status != PaymentAttemptStatusFailed {
		return "", nil
	}
	return msg.String, err
}

// ExpireUnpaidOrders cancels orders whose online payment is still pending
// after cutoff, releasing their stock and failing the payment.
func (s *Store) ExpireUnpaidOrders(ctx context.Context, cutoff time.Time) (int, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	expired := 0
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		expired = 0
		rows, err := tx.QueryContext(ctx,
			`SELECT p.id, p.order_id FROM payments p
			 WHERE p.status = 'pending' AND p.method <> 'cash_on_delivery' AND p.created_at < $1
			 ORDER BY p.order_id FOR UPDATE SKIP LOCKED`, cutoff.UTC())
		if err != nil {
			return err
		}
		type pair struct{ payment, order int }
		var stale []pair
		for rows.Next() {
			var p pair
			if err := rows.Scan(&p.payment, &p.order); err != nil {
				rows.Close()
				return err
			}
			stale = append(stale, p)
		}
		if err := rows.Close(); err != nil {
			return err
		}
		for _, p := range stale {
			current, err := lockOrderTx(ctx, tx, p.order)
			if err != nil {
				return err
			}
			if err := writeOrderStateTx(ctx, tx, p.order, current, OrderStatusCancelled, "cancelled", current.DeliveryStatus,
				"Payment was not completed in time. The order was cancelled."); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, "UPDATE payments SET status = 'failed' WHERE id = $1", p.payment); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx,
				`UPDATE payment_attempts SET status = 'failed', error_message = 'expired' WHERE payment_id = $1 AND status = 'pending'`,
				p.payment); err != nil {
				return err
			}
			expired++
		}
		return nil
	})
	return expired, err
}

func truncateText(s string, max int) string {
	if r := []rune(s); len(r) > max {
		return string(r[:max])
	}
	return s
}

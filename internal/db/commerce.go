package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/kyambuthia/sokomoko/internal/domain"
	"github.com/kyambuthia/sokomoko/internal/money"
)

var (
	ErrPaymentNotFound         = domain.ErrPaymentNotFound
	ErrCurrencyLocked          = domain.ErrCurrencyLocked
	ErrPaymentAmountMatch      = errors.New("payment amount or currency does not match")
	ErrAttemptInProgress       = errors.New("a payment attempt is already in progress")
	ErrPaymentReviewRequired   = errors.New("payment amount discrepancy is under review")
	ErrCheckoutCurrencyChanged = errors.New("checkout currency changed")
)

// UnpaidOrderTTL is how long an online order waits for payment before
// maintenance cancels it and releases its stock.
const UnpaidOrderTTL = 30 * time.Minute

// CommerceSettings are the marketplace-wide money rules.
type CommerceSettings struct {
	Currency              string
	ShippingFlat          money.Cents
	FreeShippingThreshold money.Cents // 0 disables free shipping
	TaxRateBP             int64       // basis points, 800 = 8%
}

// GetCommerceSettings returns the money settings. The migration seeds the row.
func (s *Store) GetCommerceSettings(ctx context.Context) (CommerceSettings, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	var c CommerceSettings
	var flat, free int64
	err := s.DB.QueryRowContext(ctx,
		`SELECT currency, shipping_flat_cents, free_shipping_threshold_cents, tax_rate_bp FROM commerce_settings WHERE id = 1`,
	).Scan(&c.Currency, &flat, &free, &c.TaxRateBP)
	c.ShippingFlat, c.FreeShippingThreshold = money.Cents(flat), money.Cents(free)
	return c, err
}

// UpdateCommerceSettings saves marketplace money settings for a trusted
// internal caller. Currency may change only while there are no orders or
// enabled payment providers. Call UpdateCommerceSettingsForActor when provider
// capability data is available.
func (s *Store) UpdateCommerceSettings(ctx context.Context, c CommerceSettings) error {
	return s.updateCommerceSettings(ctx, 0, c, nil, false, nil)
}

// UpdateCommerceSettingsForActor revalidates the admin in the settings transaction.
func (s *Store) UpdateCommerceSettingsForActor(ctx context.Context, actorID int, c CommerceSettings, supportedProviders []string) error {
	return s.updateCommerceSettings(ctx, actorID, c, supportedProviders, true, nil)
}

// UpdateCommerceSettingsForActorIfUnchanged saves only if settings still match
// the snapshot used to validate the submitted values.
func (s *Store) UpdateCommerceSettingsForActorIfUnchanged(ctx context.Context, actorID int, c CommerceSettings, supportedProviders []string, expected CommerceSettings) error {
	return s.updateCommerceSettings(ctx, actorID, c, supportedProviders, true, &expected)
}

func (s *Store) updateCommerceSettings(ctx context.Context, actorID int, c CommerceSettings, supportedProviders []string, requireAdmin bool, expected *CommerceSettings) error {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	return s.withTx(ctx, func(tx *sql.Tx) error {
		if requireAdmin {
			if _, _, err := lockWorkspaceActorTx(ctx, tx, actorID, RoleAdmin); err != nil {
				return err
			}
		}
		var current CommerceSettings
		var flat, free int64
		if err := tx.QueryRowContext(ctx,
			"SELECT currency, shipping_flat_cents, free_shipping_threshold_cents, tax_rate_bp FROM commerce_settings WHERE id = 1 FOR UPDATE",
		).Scan(&current.Currency, &flat, &free, &current.TaxRateBP); err != nil {
			return err
		}
		current.ShippingFlat, current.FreeShippingThreshold = money.Cents(flat), money.Cents(free)
		if expected != nil && current != *expected {
			return ErrCommerceSettingsChanged
		}
		if requireAdmin {
			if err := ensurePaymentProvidersSupportCurrencyTx(ctx, tx, supportedProviders); err != nil {
				return err
			}
		} else if current.Currency != c.Currency {
			// The compatibility API has no provider capability data. Fail closed
			// on currency changes so it cannot bypass the invariant enforced by
			// the actor-scoped API while holding the same settings-row lock.
			if err := ensurePaymentProvidersSupportCurrencyTx(ctx, tx, nil); err != nil {
				return err
			}
		}
		if current.Currency != c.Currency {
			currency, ok := money.LookupCurrency(c.Currency)
			if ok && currency.MinorDigits < 2 {
				if !c.ShippingFlat.FitsCurrency(c.Currency) || !c.FreeShippingThreshold.FitsCurrency(c.Currency) {
					return money.ErrInvalidCurrencyPrecision
				}
				var hasFractionalProduct bool
				if err := tx.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM products WHERE deleted_at IS NULL AND price_cents % 100 <> 0)").Scan(&hasFractionalProduct); err != nil {
					return err
				}
				if hasFractionalProduct {
					return money.ErrInvalidCurrencyPrecision
				}
			}
			var orders int
			if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM orders").Scan(&orders); err != nil {
				return err
			}
			if orders > 0 {
				return ErrCurrencyLocked
			}
			if _, err := releaseReservationsTx(ctx, tx, StockReservationStatusReleased, "TRUE"); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, "UPDATE checkouts SET status = 'cancelled' WHERE status = 'open'"); err != nil {
				return err
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

func (s *Store) ListPaymentProviderConfigs(ctx context.Context) ([]PaymentProviderConfig, error) {
	ctx, cancel := s.queryContext(ctx)
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

func (s *Store) GetPaymentProviderConfig(ctx context.Context, provider string) (*PaymentProviderConfig, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	return scanProviderConfig(s.DB.QueryRowContext(ctx, "SELECT "+providerColumns+" FROM payment_providers WHERE provider = $1", provider))
}

func ensurePaymentProvidersSupportCurrencyTx(ctx context.Context, tx *sql.Tx, supportedProviders []string) error {
	rows, err := tx.QueryContext(ctx, "SELECT provider FROM payment_providers WHERE enabled ORDER BY provider")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var provider string
		if err := rows.Scan(&provider); err != nil {
			return err
		}
		if !slices.Contains(supportedProviders, provider) {
			return ErrPaymentProviderCurrencyMismatch
		}
	}
	return rows.Err()
}

// SavePaymentProviderConfig upserts a provider for a trusted internal caller.
// It cannot enable a provider without capability data; use
// SavePaymentProviderConfigForActor for that operation.
func (s *Store) SavePaymentProviderConfig(ctx context.Context, c PaymentProviderConfig) error {
	return s.savePaymentProviderConfig(ctx, 0, c, nil, false)
}

// SavePaymentProviderConfigForActor revalidates the admin and provider currency in
// the transaction that writes the provider configuration.
func (s *Store) SavePaymentProviderConfigForActor(ctx context.Context, actorID int, c PaymentProviderConfig, supportedCurrencies []string) error {
	return s.savePaymentProviderConfig(ctx, actorID, c, supportedCurrencies, true)
}

func (s *Store) savePaymentProviderConfig(ctx context.Context, actorID int, c PaymentProviderConfig, supportedCurrencies []string, requireAdmin bool) error {
	settings, err := json.Marshal(c.Settings)
	if err != nil {
		return err
	}
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	return s.withTx(ctx, func(tx *sql.Tx) error {
		if requireAdmin {
			if _, _, err := lockWorkspaceActorTx(ctx, tx, actorID, RoleAdmin); err != nil {
				return err
			}
		}
		var currency string
		if err := tx.QueryRowContext(ctx, "SELECT currency FROM commerce_settings WHERE id = 1 FOR UPDATE").Scan(&currency); err != nil {
			return err
		}
		if requireAdmin && c.Enabled && !slices.Contains(supportedCurrencies, currency) {
			return ErrPaymentProviderCurrencyMismatch
		}
		if !requireAdmin && c.Enabled {
			var alreadyEnabled bool
			if err := tx.QueryRowContext(ctx,
				"SELECT EXISTS (SELECT 1 FROM payment_providers WHERE provider = $1 AND enabled)", c.Provider,
			).Scan(&alreadyEnabled); err != nil {
				return err
			}
			if !alreadyEnabled {
				return ErrPaymentProviderCurrencyMismatch
			}
		}
		if !c.Enabled {
			var anotherEnabled bool
			if err := tx.QueryRowContext(ctx,
				"SELECT EXISTS (SELECT 1 FROM payment_providers WHERE enabled AND provider <> $1)", c.Provider,
			).Scan(&anotherEnabled); err != nil {
				return err
			}
			if !anotherEnabled {
				return ErrLastPaymentMethod
			}
		}
		_, err := tx.ExecContext(ctx,
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
	})
}

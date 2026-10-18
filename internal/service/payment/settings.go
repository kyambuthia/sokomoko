package payment

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/money"
	"github.com/kyambuthia/sokomoko/internal/payments"
)

// FieldView is one provider setting as the admin form shows it. Secret
// values are never returned; IsSet says whether one is stored.
type FieldView struct {
	payments.Field
	Value string
	IsSet bool
}

// ProviderView is a provider's setup card.
type ProviderView struct {
	payments.Descriptor
	Enabled           bool
	Mode              string
	Fields            []FieldView
	SupportsCurrency  bool
	Ready             bool   // every required field has a value
	WebhookPath       string // path to paste into the provider dashboard
	CredentialsBroken bool   // stored secrets cannot be opened with SECRETS_KEY
}

// SecretsConfigured reports whether credentials can be stored.
func (s *Service) SecretsConfigured() bool { return s.box != nil }

// Providers lists every provider with its current setup.
func (s *Service) Providers() ([]ProviderView, error) {
	settings, err := s.store.GetCommerceSettings()
	if err != nil {
		return nil, err
	}
	configs, err := s.store.ListPaymentProviderConfigs()
	if err != nil {
		return nil, err
	}
	byID := map[string]db.PaymentProviderConfig{}
	for _, c := range configs {
		byID[c.Provider] = c
	}
	views := make([]ProviderView, 0, len(s.registry.All()))
	for _, p := range s.registry.All() {
		d := p.Descriptor()
		record := byID[d.ID]
		view := ProviderView{Descriptor: d, Enabled: record.Enabled, Mode: record.Mode, SupportsCurrency: d.SupportsCurrency(settings.Currency)}
		if view.Mode == "" {
			view.Mode = "test"
		}
		values := map[string]string{}
		if record.Provider != "" {
			cfg, err := s.decrypt(record)
			if err != nil {
				view.CredentialsBroken = true
				cfg = payments.Config{Values: record.Settings}
			}
			values = cfg.Values
		}
		view.Ready = true
		for _, f := range d.Fields {
			fv := FieldView{Field: f, IsSet: strings.TrimSpace(values[f.Key]) != ""}
			if !f.Secret {
				fv.Value = values[f.Key]
			}
			if f.Required && !fv.IsSet {
				view.Ready = false
			}
			view.Fields = append(view.Fields, fv)
		}
		if _, ok := p.(payments.WebhookParser); ok && d.ID == "paystack" {
			view.WebhookPath = "/payments/webhooks/" + d.ID
		}
		views = append(views, view)
	}
	return views, nil
}

// ProviderInput is a submitted provider setup form. Empty secret values keep
// the stored secret.
type ProviderInput struct {
	Enabled bool
	Mode    string
	Values  map[string]string
}

// SaveProvider stores a provider's setup after validating it.
func (s *Service) SaveProvider(actor *Actor, id string, input ProviderInput) error {
	if actor == nil || actor.Role != db.RoleAdmin {
		return ErrForbidden
	}
	provider, ok := s.registry.Get(id)
	if !ok {
		return ErrUnknownProvider
	}
	d := provider.Descriptor()
	mode := input.Mode
	if mode != "live" {
		mode = "test"
	}
	if !d.Online {
		mode = "live"
	}
	existing, err := s.store.GetPaymentProviderConfig(id)
	if err != nil {
		return err
	}
	current := map[string]string{}
	if existing != nil {
		if cfg, err := s.decrypt(*existing); err == nil {
			current = cfg.Values
		}
	}

	settings := map[string]string{}
	secrets := map[string]string{}
	secretChanged := false
	var missing []string
	for _, f := range d.Fields {
		value := strings.TrimSpace(input.Values[f.Key])
		if len(f.Options) > 0 && value != "" && !slices.Contains(f.Options, value) {
			return fmt.Errorf("%w: %s", ErrMissingFields, f.Label)
		}
		if f.Secret {
			if value == "" {
				value = current[f.Key]
			} else {
				secretChanged = true
			}
			if value != "" {
				secrets[f.Key] = value
			}
		} else if value != "" {
			settings[f.Key] = value
		}
		if f.Required && value == "" {
			missing = append(missing, f.Label)
		}
	}
	if input.Enabled {
		if len(missing) > 0 {
			return fmt.Errorf("%w: %s", ErrMissingFields, strings.Join(missing, ", "))
		}
		commerce, err := s.store.GetCommerceSettings()
		if err != nil {
			return err
		}
		if !d.SupportsCurrency(commerce.Currency) {
			return fmt.Errorf("%w: %s charges %s", ErrCurrencyMismatch, d.Name, strings.Join(d.Currencies, ", "))
		}
		if ps, ok := provider.(*payments.Paystack); ok {
			if err := ps.CheckKeyMode(payments.Config{Mode: mode, Values: secrets}); err != nil {
				return fmt.Errorf("%w: %v", ErrMissingFields, err)
			}
		}
	} else if err := s.ensureAnotherEnabled(id); err != nil {
		return err
	}

	record := db.PaymentProviderConfig{Provider: id, Enabled: input.Enabled, Mode: mode, Settings: settings}
	// Unchanged secrets stay sealed as stored (SavePaymentProviderConfig keeps
	// them when Secrets is nil).
	if secretChanged {
		if s.box == nil {
			return ErrSecretsKeyMissing
		}
		raw, err := json.Marshal(secrets)
		if err != nil {
			return err
		}
		if record.Secrets, err = s.box.Seal(raw); err != nil {
			return err
		}
	}
	if err := s.store.SavePaymentProviderConfig(record); err != nil {
		return err
	}
	state := "disabled"
	if input.Enabled {
		state = "enabled " + mode
	}
	_ = s.store.CreateAuditLog(actor.ID, "payments.provider.update", "payment_provider", 0, id+": "+state)
	return nil
}

// ensureAnotherEnabled keeps checkout usable: some other method must remain.
func (s *Service) ensureAnotherEnabled(id string) error {
	providers, err := s.enabled()
	if err != nil {
		return err
	}
	for _, ep := range providers {
		if ep.provider.Descriptor().ID != id {
			return nil
		}
	}
	return ErrLastMethod
}

// Pricing is the admin's currency and pricing form.
type Pricing struct {
	Currency              string
	ShippingFlat          money.Cents
	FreeShippingThreshold money.Cents
	TaxRateBP             int64
}

func (s *Service) Pricing() (Pricing, error) {
	c, err := s.store.GetCommerceSettings()
	return Pricing(c), err
}

// PricingInput is the submitted pricing form, as typed.
type PricingInput struct {
	Currency              string
	ShippingFlat          string
	FreeShippingThreshold string
	TaxRatePercent        string
}

// SavePricing validates and stores currency and pricing. Switching currency
// is refused when an enabled online provider cannot charge it.
func (s *Service) SavePricing(actor *Actor, input PricingInput) (Pricing, error) {
	if actor == nil || actor.Role != db.RoleAdmin {
		return Pricing{}, ErrForbidden
	}
	currency, ok := money.LookupCurrency(input.Currency)
	if !ok {
		return Pricing{}, fmt.Errorf("%w: choose a supported currency", ErrInvalidPricing)
	}
	flat, err := money.Parse(input.ShippingFlat)
	if err != nil {
		return Pricing{}, fmt.Errorf("%w: shipping fee", ErrInvalidPricing)
	}
	free := money.Cents(0)
	if strings.TrimSpace(input.FreeShippingThreshold) != "" {
		if free, err = money.Parse(input.FreeShippingThreshold); err != nil {
			return Pricing{}, fmt.Errorf("%w: free shipping threshold", ErrInvalidPricing)
		}
	}
	taxPct, err := money.Parse(input.TaxRatePercent) // "8" or "16.5" as hundredths
	if err != nil || taxPct > 10000 {
		return Pricing{}, fmt.Errorf("%w: tax rate must be 0 to 100%%", ErrInvalidPricing)
	}
	providers, err := s.enabled()
	if err != nil {
		return Pricing{}, err
	}
	for _, ep := range providers {
		if d := ep.provider.Descriptor(); !d.SupportsCurrency(currency.Code) {
			return Pricing{}, fmt.Errorf("%w: disable %s first; it charges %s", ErrCurrencyMismatch, d.Name, strings.Join(d.Currencies, ", "))
		}
	}
	settings := db.CommerceSettings{Currency: currency.Code, ShippingFlat: flat, FreeShippingThreshold: free, TaxRateBP: int64(taxPct)}
	if err := s.store.UpdateCommerceSettings(settings); err != nil {
		return Pricing{}, err
	}
	_ = s.store.CreateAuditLog(actor.ID, "payments.pricing.update", "commerce_settings", 1,
		fmt.Sprintf("currency=%s shipping=%s free=%s tax_bp=%d", currency.Code, flat, free, taxPct))
	return Pricing(settings), nil
}

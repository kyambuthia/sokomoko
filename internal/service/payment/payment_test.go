package payment

import (
	"bytes"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/db/dbtest"
	"github.com/kyambuthia/sokomoko/internal/payments"
	"github.com/kyambuthia/sokomoko/internal/secret"
)

func newTestService(t *testing.T) (*Service, *db.Store) {
	t.Helper()
	store, err := db.OpenStore(dbtest.DSN(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.ApplySchema(); err != nil {
		t.Fatal(err)
	}
	return New(store, payments.NewRegistry(payments.Options{}), secret.Development()), store
}

var admin = &Actor{ID: 1, Role: db.RoleAdmin}

func mpesaInput(enabled bool) ProviderInput {
	return ProviderInput{Enabled: enabled, Mode: "test", Values: map[string]string{
		"consumer_key": "ck", "consumer_secret": "cs-very-secret", "passkey": "pk", "shortcode": "174379",
		"transaction_type": "CustomerPayBillOnline",
	}}
}

func TestBuildRecordUsesEnabledMethods(t *testing.T) {
	svc, _ := newTestService(t)
	record, err := svc.BuildRecord(t.Context(), MethodCashOnDelivery, 4250, "USD")
	if err != nil || record.Provider != "manual_cash_on_delivery" || record.Status != db.PaymentStatusPending || record.Amount != 4250 {
		t.Fatalf("cash record = %+v, %v", record, err)
	}
	if _, err := svc.BuildRecord(t.Context(), "mpesa", 4250, "KES"); !errors.Is(err, ErrInvalidMethod) {
		t.Fatalf("disabled mpesa = %v, want ErrInvalidMethod", err)
	}
	if _, err := svc.BuildRecord(t.Context(), "wire_transfer", 20, "USD"); !errors.Is(err, ErrInvalidMethod) {
		t.Fatalf("unknown method = %v", err)
	}
}

func TestSaveProviderValidatesAndSealsSecrets(t *testing.T) {
	svc, store := newTestService(t)

	if err := svc.SaveProvider(t.Context(), &Actor{ID: 2, Role: db.RoleStaff}, "mpesa", mpesaInput(true)); !errors.Is(err, ErrForbidden) {
		t.Fatalf("staff save = %v", err)
	}
	if err := svc.SaveProvider(t.Context(), admin, "mpesa", mpesaInput(true)); !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("mpesa with USD store = %v, want ErrCurrencyMismatch", err)
	}
	if _, err := svc.SavePricing(t.Context(), admin, PricingInput{Currency: "KES", ShippingFlat: "200", FreeShippingThreshold: "0", TaxRatePercent: "16"}); err != nil {
		t.Fatal(err)
	}
	incomplete := mpesaInput(true)
	delete(incomplete.Values, "passkey")
	if err := svc.SaveProvider(t.Context(), admin, "mpesa", incomplete); !errors.Is(err, ErrMissingFields) {
		t.Fatalf("missing passkey = %v", err)
	}
	if err := svc.SaveProvider(t.Context(), admin, "mpesa", mpesaInput(true)); err != nil {
		t.Fatal(err)
	}
	record, err := store.GetPaymentProviderConfig(t.Context(), "mpesa")
	if err != nil || record == nil || len(record.Secrets) == 0 || bytes.Contains(record.Secrets, []byte("cs-very-secret")) {
		t.Fatalf("secrets not sealed: %+v %v", record, err)
	}
	if record.Settings["shortcode"] != "174379" || record.Settings["consumer_secret"] != "" {
		t.Fatalf("settings = %v", record.Settings)
	}

	// Saving with blank secret fields keeps the stored secrets.
	keep := mpesaInput(true)
	for _, k := range []string{"consumer_key", "consumer_secret", "passkey"} {
		keep.Values[k] = ""
	}
	keep.Values["shortcode"] = "600000"
	if err := svc.SaveProvider(t.Context(), admin, "mpesa", keep); err != nil {
		t.Fatalf("save without re-entering secrets = %v", err)
	}
	views, err := svc.Providers(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range views {
		if v.ID != "mpesa" {
			continue
		}
		if !v.Enabled || !v.Ready || !v.SupportsCurrency {
			t.Fatalf("mpesa view = %+v", v)
		}
		for _, f := range v.Fields {
			if f.Secret && (f.Value != "" || !f.IsSet) {
				t.Fatalf("secret field %s leaked or lost: %+v", f.Key, f)
			}
			if f.Key == "shortcode" && f.Value != "600000" {
				t.Fatalf("shortcode = %q", f.Value)
			}
		}
	}
	if !svc.IsSupportedMethod(t.Context(), "mpesa", "") || svc.IsSupportedMethod(t.Context(), "mpesa", "USD") {
		t.Fatal("mpesa support should follow the store currency")
	}

	// Switching currency away from KES is refused while M-Pesa is on.
	if _, err := svc.SavePricing(t.Context(), admin, PricingInput{Currency: "USD", ShippingFlat: "1", TaxRatePercent: "0"}); !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("currency switch with mpesa on = %v", err)
	}
}

func TestSaveProviderKeepsOneMethod(t *testing.T) {
	svc, _ := newTestService(t)
	if err := svc.SaveProvider(t.Context(), admin, "cash_on_delivery", ProviderInput{Enabled: false}); !errors.Is(err, ErrLastMethod) {
		t.Fatalf("disabling the only method = %v, want ErrLastMethod", err)
	}
}

func TestSaveProviderNeedsSecretsKey(t *testing.T) {
	svc, _ := newTestService(t)
	svc.box = nil
	if _, err := svc.SavePricing(t.Context(), admin, PricingInput{Currency: "NGN", ShippingFlat: "0", TaxRatePercent: "7.5"}); err != nil {
		t.Fatal(err)
	}
	err := svc.SaveProvider(t.Context(), admin, "paystack", ProviderInput{Enabled: true, Mode: "test", Values: map[string]string{"secret_key": "sk_test_x"}})
	if !errors.Is(err, ErrSecretsKeyMissing) {
		t.Fatalf("save without key = %v", err)
	}
	if err := svc.SaveProvider(t.Context(), admin, "paystack", ProviderInput{Enabled: true, Mode: "live", Values: map[string]string{"secret_key": "sk_test_x"}}); !errors.Is(err, ErrMissingFields) {
		t.Fatalf("live mode with a test key = %v", err)
	}
}

func TestSavePricingParsesInput(t *testing.T) {
	svc, _ := newTestService(t)
	saved, err := svc.SavePricing(t.Context(), admin, PricingInput{Currency: "kes", ShippingFlat: "250", FreeShippingThreshold: "5000", TaxRatePercent: "16.5"})
	if err != nil || saved.Currency != "KES" || saved.ShippingFlat != 25000 || saved.FreeShippingThreshold != 500000 || saved.TaxRateBP != 1650 {
		t.Fatalf("SavePricing = %+v, %v", saved, err)
	}
	for _, bad := range []PricingInput{
		{Currency: "XXX", ShippingFlat: "1", TaxRatePercent: "1"},
		{Currency: "USD", ShippingFlat: "-1", TaxRatePercent: "1"},
		{Currency: "USD", ShippingFlat: "1", TaxRatePercent: "101"},
	} {
		if _, err := svc.SavePricing(t.Context(), admin, bad); !errors.Is(err, ErrInvalidPricing) {
			t.Errorf("SavePricing(%+v) = %v, want ErrInvalidPricing", bad, err)
		}
	}
}

func TestValidateProductionPaymentReadiness(t *testing.T) {
	svc, _ := newTestService(t)
	if err := svc.ValidateProduction(t.Context(), ""); err != nil {
		t.Fatalf("cash only startup = %v", err)
	}
	if _, err := svc.SavePricing(t.Context(), admin, PricingInput{Currency: "NGN", ShippingFlat: "0", TaxRatePercent: "0"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveProvider(t.Context(), admin, "paystack", ProviderInput{
		Enabled: true, Mode: "test", Values: map[string]string{"secret_key": "sk_test_ready"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.ValidateProduction(t.Context(), ""); !errors.Is(err, ErrNoPublicURL) {
		t.Fatalf("missing public URL = %v", err)
	}
	if err := svc.ValidateProduction(t.Context(), "https://shop.example"); err != nil {
		t.Fatalf("ready provider = %v", err)
	}
	svc.box = nil
	if err := svc.ValidateProduction(t.Context(), "https://shop.example"); !errors.Is(err, ErrSecretsKeyMissing) {
		t.Fatalf("missing secrets key = %v", err)
	}
	wrong, err := secret.New(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	svc.box = wrong
	if err := svc.ValidateProduction(t.Context(), "https://shop.example"); !errors.Is(err, secret.ErrCorrupted) {
		t.Fatalf("wrong secrets key = %v", err)
	}
}

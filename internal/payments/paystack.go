package payments

import (
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/money"
)

const paystackAPI = "https://api.paystack.co"

// Paystack collects card, bank, USSD, and mobile money payments on a hosted
// Paystack page.
//
// Flow: Start initializes a transaction and returns Paystack's
// authorization_url. The customer pays there and Paystack redirects back to
// ReturnURL with ?reference=. Paystack also posts a charge.success webhook
// signed with HMAC-SHA512 of the body keyed by the secret key. Either way the
// payment is only captured after Verify confirms it with Paystack.
type Paystack struct {
	Client  *http.Client
	BaseURL string // tests override; defaults to the Paystack API
}

func (p *Paystack) Descriptor() Descriptor {
	return Descriptor{
		ID:         "paystack",
		Name:       "Paystack",
		Summary:    "Cards, bank transfer, USSD, and mobile money on a hosted Paystack page.",
		Countries:  []string{"NG", "GH", "ZA", "KE", "CI"},
		Currencies: []string{"NGN", "GHS", "ZAR", "KES", "USD"},
		Online:     true,
		Fields: []Field{
			{Key: "secret_key", Label: "Secret key", Secret: true, Required: true,
				Placeholder: "sk_test_… or sk_live_…",
				Help:        "Paystack dashboard → Settings → API Keys & Webhooks. Use the sk_test_ key in test mode."},
		},
		DocsURL: "https://paystack.com/docs/payments/accept-payments/",
	}
}

func (p *Paystack) base() string {
	if p.BaseURL != "" {
		return strings.TrimRight(p.BaseURL, "/")
	}
	return paystackAPI
}

func (p *Paystack) auth(cfg Config) (map[string]string, error) {
	key := cfg.Value("secret_key")
	if key == "" {
		return nil, ErrNotConfigured
	}
	return map[string]string{"Authorization": "Bearer " + key}, nil
}

// CheckKeyMode reports a mismatch between the key prefix and the mode, which
// is an easy mistake when switching to live.
func (p *Paystack) CheckKeyMode(cfg Config) error {
	key := cfg.Value("secret_key")
	switch {
	case key == "":
		return nil
	case cfg.Live() && strings.HasPrefix(key, "sk_test_"):
		return fmt.Errorf("live mode needs an sk_live_ key")
	case !cfg.Live() && strings.HasPrefix(key, "sk_live_"):
		return fmt.Errorf("test mode needs an sk_test_ key")
	}
	return nil
}

type paystackEnvelope struct {
	Status  bool            `json:"status"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func (p *Paystack) Start(ctx context.Context, cfg Config, req StartRequest) (StartResult, error) {
	headers, err := p.auth(cfg)
	if err != nil {
		return StartResult{}, err
	}
	if !p.Descriptor().SupportsCurrency(req.Currency) {
		return StartResult{}, fmt.Errorf("%w: Paystack does not charge %s", ErrUnsupported, req.Currency)
	}
	body := map[string]any{
		"email":        req.Email,
		"amount":       int64(req.Amount), // minor units (kobo, pesewas, cents)
		"currency":     strings.ToUpper(req.Currency),
		"reference":    req.Reference,
		"callback_url": req.ReturnURL,
		"metadata": map[string]any{
			"payment_id":    req.PaymentID,
			"custom_fields": []map[string]string{{"display_name": "Order", "variable_name": "order", "value": req.Description}},
		},
	}
	var env paystackEnvelope
	status, err := doJSON(ctx, p.Client, http.MethodPost, p.base()+"/transaction/initialize", headers, body, &env)
	if err != nil {
		return StartResult{}, err
	}
	if status != http.StatusOK || !env.Status {
		return StartResult{}, fmt.Errorf("%w: %s", ErrProviderFailure, firstNonEmpty(env.Message, http.StatusText(status)))
	}
	var data struct {
		AuthorizationURL string `json:"authorization_url"`
		AccessCode       string `json:"access_code"`
		Reference        string `json:"reference"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil || data.AuthorizationURL == "" {
		return StartResult{}, fmt.Errorf("%w: missing authorization URL", ErrProviderFailure)
	}
	if u, err := url.Parse(data.AuthorizationURL); err != nil || u.Scheme != "https" && p.BaseURL == "" {
		return StartResult{}, fmt.Errorf("%w: unexpected authorization URL", ErrProviderFailure)
	}
	return StartResult{
		RedirectURL: data.AuthorizationURL,
		Data:        map[string]string{"access_code": data.AccessCode},
		Charged:     req.Amount,
	}, nil
}

func (p *Paystack) Verify(ctx context.Context, cfg Config, req VerifyRequest) (Verification, error) {
	headers, err := p.auth(cfg)
	if err != nil {
		return Verification{}, err
	}
	var env paystackEnvelope
	status, err := doJSON(ctx, p.Client, http.MethodGet, p.base()+"/transaction/verify/"+url.PathEscape(req.Reference), headers, nil, &env)
	if err != nil {
		return Verification{}, err
	}
	if status == http.StatusNotFound {
		return Verification{Status: StatusPending}, nil
	}
	if status != http.StatusOK || !env.Status {
		return Verification{}, fmt.Errorf("%w: %s", ErrProviderFailure, firstNonEmpty(env.Message, http.StatusText(status)))
	}
	var data struct {
		Status          string `json:"status"`
		Amount          int64  `json:"amount"`
		Currency        string `json:"currency"`
		Reference       string `json:"reference"`
		GatewayResponse string `json:"gateway_response"`
		ID              int64  `json:"id"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		return Verification{}, fmt.Errorf("%w: decode verification: %v", ErrProviderFailure, err)
	}
	if data.Reference != "" && data.Reference != req.Reference {
		return Verification{}, fmt.Errorf("%w: reference mismatch", ErrProviderFailure)
	}
	v := Verification{Amount: money.Cents(data.Amount), Currency: strings.ToUpper(data.Currency), Receipt: strconv.FormatInt(data.ID, 10)}
	switch data.Status {
	case "success":
		v.Status = StatusPaid
	case "failed", "reversed":
		v.Status = StatusFailed
		v.Reason = firstNonEmpty(data.GatewayResponse, "The payment was declined.")
	case "abandoned":
		// Paystack marks a transaction abandoned until the customer pays, so
		// it is still payable from the same page.
		v.Status = StatusPending
	default:
		v.Status = StatusPending
	}
	return v, nil
}

func (p *Paystack) CreateRefund(ctx context.Context, cfg Config, reference string, amount money.Cents, currency string) (RefundResult, error) {
	headers, err := p.auth(cfg)
	if err != nil {
		return RefundResult{}, err
	}
	var env paystackEnvelope
	status, err := doJSON(ctx, p.Client, http.MethodPost, p.base()+"/refund", headers,
		map[string]any{"transaction": reference, "amount": int64(amount), "currency": currency,
			"merchant_note": "Sokomoko confirmed excess charge"}, &env)
	if err != nil {
		return RefundResult{}, err
	}
	if status != http.StatusOK || !env.Status {
		return RefundResult{}, fmt.Errorf("%w: refund request: %s", ErrProviderFailure, firstNonEmpty(env.Message, http.StatusText(status)))
	}
	result, err := parsePaystackRefund(env.Data)
	if err != nil {
		return RefundResult{}, err
	}
	if result.Amount != amount || !strings.EqualFold(result.Currency, currency) {
		return RefundResult{}, fmt.Errorf("%w: refund amount or currency mismatch", ErrProviderFailure)
	}
	return result, nil
}

func (p *Paystack) FetchRefund(ctx context.Context, cfg Config, refundID string) (RefundResult, error) {
	headers, err := p.auth(cfg)
	if err != nil {
		return RefundResult{}, err
	}
	var env paystackEnvelope
	status, err := doJSON(ctx, p.Client, http.MethodGet, p.base()+"/refund/"+url.PathEscape(refundID), headers, nil, &env)
	if err != nil {
		return RefundResult{}, err
	}
	if status != http.StatusOK || !env.Status {
		return RefundResult{}, fmt.Errorf("%w: refund lookup: %s", ErrProviderFailure, firstNonEmpty(env.Message, http.StatusText(status)))
	}
	return parsePaystackRefund(env.Data)
}

func parsePaystackRefund(raw json.RawMessage) (RefundResult, error) {
	var data struct {
		ID       int64  `json:"id"`
		Status   string `json:"status"`
		Amount   int64  `json:"amount"`
		Currency string `json:"currency"`
	}
	if err := json.Unmarshal(raw, &data); err != nil || data.ID <= 0 || data.Amount <= 0 || data.Currency == "" {
		return RefundResult{}, fmt.Errorf("%w: invalid refund response", ErrProviderFailure)
	}
	return RefundResult{ID: strconv.FormatInt(data.ID, 10), Status: data.Status,
		Amount: money.Cents(data.Amount), Currency: strings.ToUpper(data.Currency)}, nil
}

// ParseWebhook authenticates a Paystack event. Only the reference is trusted
// from the body; the caller confirms the result with Verify.
func (p *Paystack) ParseWebhook(cfg Config, r *http.Request, body []byte) (WebhookEvent, error) {
	key := cfg.Value("secret_key")
	if key == "" {
		return WebhookEvent{}, ErrNotConfigured
	}
	mac := hmac.New(sha512.New, []byte(key))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(strings.ToLower(strings.TrimSpace(r.Header.Get("X-Paystack-Signature"))))) {
		return WebhookEvent{}, ErrBadSignature
	}
	var event struct {
		Event string `json:"event"`
		Data  struct {
			Reference string `json:"reference"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &event); err != nil {
		return WebhookEvent{}, fmt.Errorf("decode webhook: %w", err)
	}
	if !strings.HasPrefix(event.Event, "charge.") || event.Data.Reference == "" {
		return WebhookEvent{}, ErrUnsupported
	}
	return WebhookEvent{Reference: event.Data.Reference}, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

package payments

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kyambuthia/sokomoko/internal/money"
)

const (
	mpesaSandboxAPI    = "https://sandbox.safaricom.co.ke"
	mpesaProductionAPI = "https://api.safaricom.co.ke"
	mpesaPayBill       = "CustomerPayBillOnline"
	mpesaBuyGoods      = "CustomerBuyGoodsOnline"
)

// Kenya does not observe daylight saving, and the distroless runtime image
// ships no tz database, so EAT is a fixed zone.
var eat = time.FixedZone("EAT", 3*60*60)

// Mpesa collects payments with Safaricom Daraja's Lipa na M-Pesa Online (STK
// push): the customer gets a PIN prompt on their phone.
//
// Flow: Start obtains an OAuth token and sends the STK push. Safaricom later
// posts the result to CallbackURL. Daraja callbacks are not signed, so the
// callback URL carries a per-payment secret token, and the result is
// confirmed with the STK push query API before the payment is captured.
// M-Pesa charges whole shillings, so totals are rounded up.
type Mpesa struct {
	Client  *http.Client
	BaseURL string // tests override; otherwise chosen by mode

	mu     sync.Mutex
	tokens map[string]cachedToken
}

type cachedToken struct {
	value   string
	expires time.Time
}

func (m *Mpesa) Descriptor() Descriptor {
	return Descriptor{
		ID:         "mpesa",
		Name:       "M-Pesa (Daraja)",
		Summary:    "Lipa na M-Pesa: the customer approves the payment on their phone.",
		Countries:  []string{"KE"},
		Currencies: []string{"KES"},
		Online:     true,
		NeedsPhone: true,
		Fields: []Field{
			{Key: "consumer_key", Label: "Consumer key", Secret: true, Required: true,
				Help: "Daraja portal → My Apps. Use the sandbox app's keys in test mode."},
			{Key: "consumer_secret", Label: "Consumer secret", Secret: true, Required: true},
			{Key: "passkey", Label: "Lipa na M-Pesa passkey", Secret: true, Required: true,
				Help: "Sandbox passkey is on the Daraja test credentials page; Safaricom emails the live one."},
			{Key: "shortcode", Label: "Business short code", Required: true, Placeholder: "174379",
				Help: "Paybill number, or the store number (head office) for a Buy Goods till."},
			{Key: "transaction_type", Label: "Account type", Required: true, Options: []string{mpesaPayBill, mpesaBuyGoods},
				Help: "CustomerPayBillOnline for a Paybill, CustomerBuyGoodsOnline for a till."},
			{Key: "till_number", Label: "Till number", Help: "Buy Goods only: the till that receives the money."},
			{Key: "account_reference", Label: "Account reference", Placeholder: "SOKOMOKO",
				Help: "Shown to the customer on the prompt. Up to 12 characters."},
		},
		DocsURL: "https://developer.safaricom.co.ke/APIs/MpesaExpressSimulate",
	}
}

func (m *Mpesa) base(cfg Config) string {
	if m.BaseURL != "" {
		return strings.TrimRight(m.BaseURL, "/")
	}
	if cfg.Live() {
		return mpesaProductionAPI
	}
	return mpesaSandboxAPI
}

// NormalizePhone converts Kenyan mobile numbers to the 2547XXXXXXXX or
// 2541XXXXXXXX form Daraja expects.
func NormalizePhone(raw string) (string, error) {
	var digits strings.Builder
	for _, r := range raw {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		} else if !strings.ContainsRune(" +-()", r) {
			return "", ErrInvalidPhone
		}
	}
	d := digits.String()
	switch {
	case len(d) == 10 && d[0] == '0':
		d = "254" + d[1:]
	case len(d) == 9 && (d[0] == '7' || d[0] == '1'):
		d = "254" + d
	}
	if len(d) != 12 || !strings.HasPrefix(d, "254") || (d[3] != '7' && d[3] != '1') {
		return "", ErrInvalidPhone
	}
	return d, nil
}

func (m *Mpesa) token(ctx context.Context, cfg Config) (string, error) {
	key, secret := cfg.Value("consumer_key"), cfg.Value("consumer_secret")
	if key == "" || secret == "" {
		return "", ErrNotConfigured
	}
	cacheKey := m.base(cfg) + "|" + key
	m.mu.Lock()
	if cached, ok := m.tokens[cacheKey]; ok && time.Now().Before(cached.expires) {
		m.mu.Unlock()
		return cached.value, nil
	}
	m.mu.Unlock()

	var resp struct {
		AccessToken  string `json:"access_token"`
		ExpiresIn    string `json:"expires_in"`
		ErrorMessage string `json:"errorMessage"`
	}
	basic := base64.StdEncoding.EncodeToString([]byte(key + ":" + secret))
	status, err := doJSON(ctx, m.Client, http.MethodGet, m.base(cfg)+"/oauth/v1/generate?grant_type=client_credentials",
		map[string]string{"Authorization": "Basic " + basic}, nil, &resp)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK || resp.AccessToken == "" {
		return "", fmt.Errorf("%w: M-Pesa rejected the consumer key or secret (%s)", ErrProviderFailure, firstNonEmpty(resp.ErrorMessage, http.StatusText(status)))
	}
	lifetime, _ := strconv.Atoi(resp.ExpiresIn)
	if lifetime <= 0 {
		lifetime = 3599
	}
	m.mu.Lock()
	if m.tokens == nil {
		m.tokens = map[string]cachedToken{}
	}
	m.tokens[cacheKey] = cachedToken{value: resp.AccessToken, expires: time.Now().Add(time.Duration(lifetime-60) * time.Second)}
	m.mu.Unlock()
	return resp.AccessToken, nil
}

func (m *Mpesa) password(cfg Config, now time.Time) (string, string, error) {
	shortcode, passkey := cfg.Value("shortcode"), cfg.Value("passkey")
	if shortcode == "" || passkey == "" {
		return "", "", ErrNotConfigured
	}
	timestamp := now.In(eat).Format("20060102150405")
	return base64.StdEncoding.EncodeToString([]byte(shortcode + passkey + timestamp)), timestamp, nil
}

func (m *Mpesa) Start(ctx context.Context, cfg Config, req StartRequest) (StartResult, error) {
	if !strings.EqualFold(req.Currency, "KES") {
		return StartResult{}, fmt.Errorf("%w: M-Pesa charges KES only", ErrUnsupported)
	}
	phone, err := NormalizePhone(req.Phone)
	if err != nil {
		return StartResult{}, err
	}
	token, err := m.token(ctx, cfg)
	if err != nil {
		return StartResult{}, err
	}
	password, timestamp, err := m.password(cfg, time.Now())
	if err != nil {
		return StartResult{}, err
	}
	txType := cfg.Value("transaction_type")
	if txType == "" {
		txType = mpesaPayBill
	}
	partyB := cfg.Value("shortcode")
	if txType == mpesaBuyGoods && cfg.Value("till_number") != "" {
		partyB = cfg.Value("till_number")
	}
	shillings := req.Amount.WholeUnitsCeil()
	body := map[string]any{
		"BusinessShortCode": cfg.Value("shortcode"),
		"Password":          password,
		"Timestamp":         timestamp,
		"TransactionType":   txType,
		"Amount":            shillings,
		"PartyA":            phone,
		"PartyB":            partyB,
		"PhoneNumber":       phone,
		"CallBackURL":       req.CallbackURL,
		"AccountReference":  truncate(firstNonEmpty(cfg.Value("account_reference"), "SOKOMOKO"), 12),
		"TransactionDesc":   truncate(firstNonEmpty(req.Description, "Order"), 13),
	}
	var resp struct {
		MerchantRequestID string `json:"MerchantRequestID"`
		CheckoutRequestID string `json:"CheckoutRequestID"`
		ResponseCode      string `json:"ResponseCode"`
		CustomerMessage   string `json:"CustomerMessage"`
		ErrorMessage      string `json:"errorMessage"`
	}
	status, err := doJSON(ctx, m.Client, http.MethodPost, m.base(cfg)+"/mpesa/stkpush/v1/processrequest",
		map[string]string{"Authorization": "Bearer " + token}, body, &resp)
	if err != nil {
		return StartResult{}, err
	}
	if status != http.StatusOK || resp.ResponseCode != "0" || resp.CheckoutRequestID == "" {
		return StartResult{}, fmt.Errorf("%w: %s", ErrProviderFailure, firstNonEmpty(resp.ErrorMessage, resp.CustomerMessage, http.StatusText(status)))
	}
	return StartResult{
		Data: map[string]string{
			"merchant_request_id": resp.MerchantRequestID,
			"checkout_request_id": resp.CheckoutRequestID,
			"phone":               phone,
			"charged_shillings":   strconv.FormatInt(shillings, 10),
		},
		Charged: money.Cents(shillings * 100),
	}, nil
}

// mpesaResultReasons maps common STK result codes to customer messages.
var mpesaResultReasons = map[string]string{
	"1":    "Your M-Pesa balance is too low for this payment.",
	"1032": "The payment was cancelled on the phone.",
	"1037": "The phone could not be reached. Check it is on and try again.",
	"2001": "The M-Pesa PIN was incorrect.",
	"1025": "M-Pesa could not send the prompt. Try again.",
	"1019": "The payment request expired. Try again.",
}

func mpesaReason(code, desc string) string {
	if reason, ok := mpesaResultReasons[code]; ok {
		return reason
	}
	return firstNonEmpty(desc, "M-Pesa did not complete the payment.")
}

// Verify asks Daraja for the STK push result. The query does not return the
// amount, so a successful result confirms the amount Start requested.
func (m *Mpesa) Verify(ctx context.Context, cfg Config, req VerifyRequest) (Verification, error) {
	checkoutID := req.Data["checkout_request_id"]
	if checkoutID == "" {
		return Verification{Status: StatusPending}, nil
	}
	token, err := m.token(ctx, cfg)
	if err != nil {
		return Verification{}, err
	}
	password, timestamp, err := m.password(cfg, time.Now())
	if err != nil {
		return Verification{}, err
	}
	var resp struct {
		ResponseCode string `json:"ResponseCode"`
		ResultCode   string `json:"ResultCode"`
		ResultDesc   string `json:"ResultDesc"`
		ErrorCode    string `json:"errorCode"`
		ErrorMessage string `json:"errorMessage"`
	}
	status, err := doJSON(ctx, m.Client, http.MethodPost, m.base(cfg)+"/mpesa/stkpushquery/v1/query",
		map[string]string{"Authorization": "Bearer " + token},
		map[string]any{"BusinessShortCode": cfg.Value("shortcode"), "Password": password, "Timestamp": timestamp, "CheckoutRequestID": checkoutID},
		&resp)
	if err != nil {
		return Verification{}, err
	}
	// While the customer is still deciding, Daraja answers with an error
	// such as 500.001.1001 "The transaction is being processed".
	if resp.ErrorCode != "" || status != http.StatusOK {
		return Verification{Status: StatusPending}, nil
	}
	if resp.ResultCode == "0" {
		return Verification{Status: StatusPaid, Amount: req.Charged, Currency: "KES"}, nil
	}
	return Verification{Status: StatusFailed, Reason: mpesaReason(resp.ResultCode, resp.ResultDesc)}, nil
}

// ParseWebhook reads Daraja's STK callback. It is not signed: the caller must
// have authenticated the per-payment callback token and must confirm the
// result with Verify before capturing.
func (m *Mpesa) ParseWebhook(_ Config, _ *http.Request, body []byte) (WebhookEvent, error) {
	var payload struct {
		Body struct {
			StkCallback struct {
				MerchantRequestID string      `json:"MerchantRequestID"`
				CheckoutRequestID string      `json:"CheckoutRequestID"`
				ResultCode        json.Number `json:"ResultCode"`
				ResultDesc        string      `json:"ResultDesc"`
				CallbackMetadata  struct {
					Item []struct {
						Name  string `json:"Name"`
						Value any    `json:"Value"`
					} `json:"Item"`
				} `json:"CallbackMetadata"`
			} `json:"stkCallback"`
		} `json:"Body"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return WebhookEvent{}, fmt.Errorf("decode callback: %w", err)
	}
	cb := payload.Body.StkCallback
	if cb.CheckoutRequestID == "" {
		return WebhookEvent{}, ErrUnsupported
	}
	result := &Verification{Currency: "KES"}
	if cb.ResultCode.String() == "0" {
		result.Status = StatusPaid
		for _, item := range cb.CallbackMetadata.Item {
			switch item.Name {
			case "Amount":
				if v, ok := item.Value.(float64); ok {
					result.Amount = money.Cents(int64(v * 100))
				}
			case "MpesaReceiptNumber":
				result.Receipt = fmt.Sprint(item.Value)
			}
		}
	} else {
		result.Status = StatusFailed
		result.Reason = mpesaReason(cb.ResultCode.String(), cb.ResultDesc)
	}
	return WebhookEvent{
		Data:   map[string]string{"checkout_request_id": cb.CheckoutRequestID, "merchant_request_id": cb.MerchantRequestID},
		Result: result,
	}, nil
}

func truncate(s string, max int) string {
	if r := []rune(s); len(r) > max {
		return string(r[:max])
	}
	return s
}

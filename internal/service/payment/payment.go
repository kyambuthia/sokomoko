// Package payment is the store's payment service. It decides which payment
// methods checkout offers, records payments for new orders, starts online
// payments with the configured provider, and confirms results from provider
// redirects, webhooks, callbacks, and status polling. It also manages the
// admin's provider setup and the store's currency and pricing.
package payment

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/money"
	"github.com/kyambuthia/sokomoko/internal/payments"
	"github.com/kyambuthia/sokomoko/internal/secret"
)

const (
	MethodCashOnDelivery = "cash_on_delivery"
	providerCashManual   = "manual_cash_on_delivery"

	// verifyInterval throttles status polls that ask the provider directly.
	verifyInterval = 5 * time.Second
)

var (
	ErrInvalidMethod     = errors.New("invalid payment method")
	ErrForbidden         = errors.New("forbidden payment settings change")
	ErrUnknownProvider   = errors.New("unknown payment provider")
	ErrMissingFields     = errors.New("required payment fields are missing")
	ErrCurrencyMismatch  = errors.New("provider does not support the store currency")
	ErrSecretsKeyMissing = errors.New("SECRETS_KEY is required to store payment credentials")
	ErrLastMethod        = errors.New("at least one payment method must stay enabled")
	ErrInvalidPricing    = errors.New("invalid pricing")
	ErrNotPayable        = errors.New("this order cannot be paid online")
	ErrNoPublicURL       = errors.New("PUBLIC_BASE_URL is required for online payments")
)

type store interface {
	CreateAuditLog(actorUserID int, action, targetType string, targetID int, details string) error
	GetCommerceSettings() (db.CommerceSettings, error)
	UpdateCommerceSettings(db.CommerceSettings) error
	ListPaymentProviderConfigs() ([]db.PaymentProviderConfig, error)
	GetPaymentProviderConfig(provider string) (*db.PaymentProviderConfig, error)
	SavePaymentProviderConfig(db.PaymentProviderConfig) error
	GetOrderPayment(userID, orderID int) (*db.OnlinePayment, error)
	GetPaymentByReference(provider, reference string) (*db.OnlinePayment, error)
	GetPaymentByCallbackToken(provider, tokenHash string) (*db.OnlinePayment, error)
	StartPaymentAttempt(paymentID int, reference, callbackTokenHash string, payload map[string]string) (int, error)
	UpdatePaymentPayload(attemptID int, payload map[string]string) error
	PaymentPayload(attemptID int) (map[string]string, error)
	CapturePayment(attemptID int, confirmed money.Cents, currency, receipt string) (bool, error)
	FailPaymentAttempt(attemptID int, reason string) error
	LastPaymentError(paymentID int) (string, error)
	ListPaymentExceptions(limit int) ([]db.PaymentException, error)
	GetPaymentException(id int) (*db.PaymentException, error)
	BeginRefund(id int) (*db.PaymentException, bool, error)
	SetPaymentExceptionStatus(id int, status, refundReference, lastError string) error
	PaymentExceptionCounts(ctx context.Context) (open, pending, review int, err error)
}

var _ store = (*db.Store)(nil)

// Actor is the admin making a settings change.
type Actor struct {
	ID   int
	Role string
}

type Service struct {
	store    store
	registry *payments.Registry
	box      *secret.Box

	mu       sync.Mutex
	verified map[string]time.Time // attempt reference -> last provider check
}

func New(store store, registry *payments.Registry, box *secret.Box) *Service {
	return &Service{store: store, registry: registry, box: box, verified: map[string]time.Time{}}
}

// GenerateIdempotencyKey returns a random checkout token.
func (s *Service) GenerateIdempotencyKey() (string, error) {
	return randomToken(18)
}

func randomToken(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// MethodOption is a payment method checkout can offer.
type MethodOption struct {
	Value      string
	Label      string
	Summary    string
	NeedsPhone bool
	Online     bool
	TestMode   bool
}

type enabledProvider struct {
	provider payments.Provider
	config   payments.Config
	record   db.PaymentProviderConfig
}

// enabled returns the providers an admin has switched on, decrypted.
func (s *Service) enabled() ([]enabledProvider, error) {
	configs, err := s.store.ListPaymentProviderConfigs()
	if err != nil {
		return nil, err
	}
	byID := map[string]db.PaymentProviderConfig{}
	for _, c := range configs {
		byID[c.Provider] = c
	}
	var out []enabledProvider
	for _, p := range s.registry.All() {
		record, ok := byID[p.Descriptor().ID]
		if !ok || !record.Enabled {
			continue
		}
		cfg, err := s.decrypt(record)
		if err != nil {
			return nil, fmt.Errorf("%s credentials: %w", record.Provider, err)
		}
		out = append(out, enabledProvider{provider: p, config: cfg, record: record})
	}
	return out, nil
}

func (s *Service) decrypt(record db.PaymentProviderConfig) (payments.Config, error) {
	values := map[string]string{}
	for k, v := range record.Settings {
		values[k] = v
	}
	if len(record.Secrets) > 0 {
		if s.box == nil {
			return payments.Config{}, ErrSecretsKeyMissing
		}
		plain, err := s.box.Open(record.Secrets)
		if err != nil {
			return payments.Config{}, err
		}
		secrets := map[string]string{}
		if err := json.Unmarshal(plain, &secrets); err != nil {
			return payments.Config{}, err
		}
		for k, v := range secrets {
			values[k] = v
		}
	}
	return payments.Config{Mode: record.Mode, Values: values}, nil
}

// ValidateProduction rejects an enabled online method that cannot process or
// reconcile payments with this instance's configuration. It is called before
// the HTTP listener starts, so a bad deployment does not silently hide a
// payment method already offered to customers.
func (s *Service) ValidateProduction(publicBaseURL string) error {
	settings, err := s.store.GetCommerceSettings()
	if err != nil {
		return err
	}
	configs, err := s.store.ListPaymentProviderConfigs()
	if err != nil {
		return err
	}
	for _, record := range configs {
		if !record.Enabled {
			continue
		}
		provider, ok := s.registry.Get(record.Provider)
		if !ok {
			return fmt.Errorf("enabled payment provider %q is unavailable", record.Provider)
		}
		descriptor := provider.Descriptor()
		if !descriptor.Online {
			continue
		}
		if strings.TrimSpace(publicBaseURL) == "" {
			return ErrNoPublicURL
		}
		if !descriptor.SupportsCurrency(settings.Currency) {
			return fmt.Errorf("%w: %s cannot charge %s", ErrCurrencyMismatch, descriptor.Name, settings.Currency)
		}
		cfg, err := s.decrypt(record)
		if err != nil {
			return fmt.Errorf("%s credentials: %w", record.Provider, err)
		}
		for _, field := range descriptor.Fields {
			if field.Required && strings.TrimSpace(cfg.Value(field.Key)) == "" {
				return fmt.Errorf("%w: %s %s", ErrMissingFields, descriptor.Name, field.Label)
			}
		}
	}
	return nil
}

// MethodOptions lists the methods checkout offers for the store currency.
func (s *Service) MethodOptions() []MethodOption {
	settings, err := s.store.GetCommerceSettings()
	if err != nil {
		slog.Error("load commerce settings", "error", err)
		return nil
	}
	providers, err := s.enabled()
	if err != nil {
		slog.Error("load payment providers", "error", err)
		return nil
	}
	var options []MethodOption
	for _, ep := range providers {
		d := ep.provider.Descriptor()
		if !d.SupportsCurrency(settings.Currency) {
			continue
		}
		options = append(options, MethodOption{
			Value: d.ID, Label: d.Name, Summary: d.Summary, NeedsPhone: d.NeedsPhone, Online: d.Online,
			TestMode: d.Online && !ep.config.Live(),
		})
	}
	return options
}

// IsSupportedMethod reports whether checkout may use method. An empty
// currency means the store currency.
func (s *Service) IsSupportedMethod(method, currency string) bool {
	method = strings.TrimSpace(strings.ToLower(method))
	if currency == "" {
		settings, err := s.store.GetCommerceSettings()
		if err != nil {
			return false
		}
		currency = settings.Currency
	}
	providers, err := s.enabled()
	if err != nil {
		return false
	}
	for _, ep := range providers {
		if ep.provider.Descriptor().ID == method && ep.provider.Descriptor().SupportsCurrency(currency) {
			return true
		}
	}
	return false
}

// MethodLabel names a method for display, including historical ones.
func (s *Service) MethodLabel(method string) string {
	if p, ok := s.registry.Get(strings.ToLower(strings.TrimSpace(method))); ok {
		return p.Descriptor().Name
	}
	return "Cash on delivery"
}

// IsOnline reports whether method collects money online.
func (s *Service) IsOnline(method string) bool {
	p, ok := s.registry.Get(strings.ToLower(strings.TrimSpace(method)))
	return ok && p.Descriptor().Online
}

// BuildRecord prepares the payment row stored with a new order. Online
// payments start pending and are captured when the provider confirms them.
func (s *Service) BuildRecord(method string, totalAmount money.Cents, currency string) (db.PaymentRecordInput, error) {
	method = strings.TrimSpace(strings.ToLower(method))
	if !s.IsSupportedMethod(method, currency) {
		return db.PaymentRecordInput{}, ErrInvalidMethod
	}
	record := db.PaymentRecordInput{Method: method, Currency: currency, Amount: totalAmount, Status: db.PaymentStatusPending}
	if method == MethodCashOnDelivery {
		record.Provider = providerCashManual
	} else {
		record.Provider = method
	}
	return record, nil
}

// StartRequest is a customer's request to pay for an order online.
type StartRequest struct {
	UserID  int
	OrderID int
	Phone   string
	BaseURL string // public origin for return and callback URLs
}

// StartOutcome says where the customer goes next.
type StartOutcome struct {
	RedirectURL string // hosted payment page, for redirect providers
}

// Start begins (or retries) an online payment for the customer's order.
func (s *Service) Start(ctx context.Context, req StartRequest) (StartOutcome, error) {
	payment, err := s.store.GetOrderPayment(req.UserID, req.OrderID)
	if err != nil {
		return StartOutcome{}, err
	}
	if payment.Status != db.PaymentStatusPending || payment.OrderStatus == db.OrderStatusCancelled || !s.IsOnline(payment.Method) {
		return StartOutcome{}, ErrNotPayable
	}
	if strings.TrimSpace(req.BaseURL) == "" {
		return StartOutcome{}, ErrNoPublicURL
	}
	provider, cfg, err := s.providerFor(payment.Provider)
	if err != nil {
		return StartOutcome{}, err
	}
	if payment.AttemptStatus == db.PaymentStatusPending {
		payload, err := s.store.PaymentPayload(payment.AttemptID)
		if err != nil {
			return StartOutcome{}, err
		}
		return StartOutcome{RedirectURL: payload["redirect_url"]}, nil
	}
	suffix, err := randomToken(6)
	if err != nil {
		return StartOutcome{}, err
	}
	token, err := randomToken(24)
	if err != nil {
		return StartOutcome{}, err
	}
	reference := fmt.Sprintf("skm-%d-%s", payment.ID, suffix)
	base := strings.TrimRight(req.BaseURL, "/")
	initial := map[string]string{}
	if payment.Provider == "mpesa" {
		initial["charged_cents"] = strconv.FormatInt(payment.Amount.WholeUnitsCeil()*100, 10)
		initial["phone"] = req.Phone
	}
	attemptID, err := s.store.StartPaymentAttempt(payment.ID, reference, hashToken(token), initial)
	if errors.Is(err, db.ErrAttemptInProgress) {
		current, lookupErr := s.store.GetOrderPayment(req.UserID, req.OrderID)
		if lookupErr != nil {
			return StartOutcome{}, lookupErr
		}
		payload, lookupErr := s.store.PaymentPayload(current.AttemptID)
		if lookupErr != nil {
			return StartOutcome{}, lookupErr
		}
		return StartOutcome{RedirectURL: payload["redirect_url"]}, nil
	}
	if err != nil {
		return StartOutcome{}, err
	}
	result, err := provider.Start(ctx, cfg, payments.StartRequest{
		PaymentID:   payment.ID,
		Reference:   reference,
		Amount:      payment.Amount,
		Currency:    payment.Currency,
		Email:       payment.UserEmail,
		Phone:       req.Phone,
		Description: "Order " + strconv.Itoa(payment.OrderID),
		ReturnURL:   base + "/payments/return",
		CallbackURL: base + "/payments/callback/" + payment.Provider + "/" + token,
	})
	if err != nil {
		_ = s.store.FailPaymentAttempt(attemptID, customerReason(err))
		return StartOutcome{}, err
	}
	data := result.Data
	if data == nil {
		data = map[string]string{}
	}
	if result.Charged > 0 {
		data["charged_cents"] = strconv.FormatInt(int64(result.Charged), 10)
	}
	if result.RedirectURL != "" {
		data["redirect_url"] = result.RedirectURL
	}
	if err := s.store.UpdatePaymentPayload(attemptID, data); err != nil {
		return StartOutcome{}, err
	}
	return StartOutcome{RedirectURL: result.RedirectURL}, nil
}

func (s *Service) providerFor(id string) (payments.Provider, payments.Config, error) {
	provider, ok := s.registry.Get(id)
	if !ok {
		return nil, payments.Config{}, ErrUnknownProvider
	}
	record, err := s.store.GetPaymentProviderConfig(id)
	if err != nil {
		return nil, payments.Config{}, err
	}
	if record == nil {
		return nil, payments.Config{}, payments.ErrNotConfigured
	}
	cfg, err := s.decrypt(*record)
	return provider, cfg, err
}

// customerReason turns provider errors into text safe to show customers.
func customerReason(err error) string {
	switch {
	case errors.Is(err, payments.ErrInvalidPhone):
		return "Enter a valid M-Pesa number, for example 0712 345 678."
	case errors.Is(err, payments.ErrNotConfigured):
		return "This payment method is not set up yet."
	case errors.Is(err, payments.ErrUnsupported):
		return "This payment method cannot charge this order."
	default:
		return "The payment could not be started. Try again."
	}
}

// CustomerReason exposes customerReason to handlers.
func CustomerReason(err error) string { return customerReason(err) }

// StatusView is what the customer sees while paying.
type StatusView struct {
	OrderID    int
	Method     string
	MethodName string
	Status     string // "pending", "paid", "failed", "cancelled", "offline"
	Message    string
	Amount     money.Cents
	Charged    money.Cents // what the provider requests, when rounded
	Currency   string
	NeedsPhone bool
	Phone      string
	Attempted  bool // a provider request has been made
}

// Status reports the payment state for the customer's order. For a pending
// attempt it asks the provider directly (throttled), so payments complete
// even when the provider cannot reach this server, as in local development.
func (s *Service) Status(ctx context.Context, userID, orderID int) (StatusView, error) {
	payment, err := s.store.GetOrderPayment(userID, orderID)
	if err != nil {
		return StatusView{}, err
	}
	view := StatusView{
		OrderID: orderID, Method: payment.Method, MethodName: s.MethodLabel(payment.Method),
		Amount: payment.Amount, Currency: payment.Currency, Attempted: payment.ExternalReference != "",
	}
	if p, ok := s.registry.Get(payment.Method); ok {
		view.NeedsPhone = p.Descriptor().NeedsPhone
	}
	payload, _ := s.store.PaymentPayload(payment.AttemptID)
	view.Phone = payload["phone"]
	if charged, err := strconv.ParseInt(payload["charged_cents"], 10, 64); err == nil {
		view.Charged = money.Cents(charged)
	}

	switch {
	case !s.IsOnline(payment.Method):
		view.Status = "offline"
		return view, nil
	case payment.OrderStatus == db.OrderStatusCancelled && payment.Status == db.PaymentStatusRefunded:
		view.Status = "cancelled"
		view.Message = "The payment was refunded after this order was cancelled."
		return view, nil
	case payment.OrderStatus == db.OrderStatusCancelled && payment.Status == db.PaymentStatusCaptured:
		view.Status = "cancelled"
		view.Message = "Payment arrived after this order was cancelled. Contact the store for a refund."
		return view, nil
	case payment.Status == db.PaymentStatusCaptured:
		view.Status = "paid"
		return view, nil
	case payment.OrderStatus == db.OrderStatusCancelled || payment.Status == db.PaymentStatusFailed:
		view.Status = "cancelled"
		view.Message = "This order was cancelled because payment was not completed."
		return view, nil
	}
	view.Status = "pending"
	if view.Attempted && s.dueForVerify(payment.ExternalReference) {
		if err := s.verifyAndApply(ctx, payment, payload); err != nil {
			slog.Warn("payment verification failed", "payment_id", payment.ID, "error", err)
		}
		if refreshed, err := s.store.GetOrderPayment(userID, orderID); err == nil && refreshed.Status == db.PaymentStatusCaptured {
			view.Status = "paid"
			return view, nil
		}
	}
	if reason, err := s.store.LastPaymentError(payment.ID); err == nil && reason != "" {
		view.Status = "failed"
		view.Message = reason
	}
	return view, nil
}

// dueForVerify throttles provider checks per attempt. A retry has a new
// reference, so it is checked straight away.
func (s *Service) dueForVerify(reference string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if last, ok := s.verified[reference]; ok && time.Since(last) < verifyInterval {
		return false
	}
	if len(s.verified) > 10000 {
		s.verified = map[string]time.Time{}
	}
	s.verified[reference] = time.Now()
	return true
}

// verifyAndApply asks the provider for the result and records it.
func (s *Service) verifyAndApply(ctx context.Context, payment *db.OnlinePayment, payload map[string]string) error {
	provider, cfg, err := s.providerFor(payment.Provider)
	if err != nil {
		return err
	}
	charged := payment.Amount
	if c, err := strconv.ParseInt(payload["charged_cents"], 10, 64); err == nil && c > 0 {
		charged = money.Cents(c)
	}
	v, err := provider.Verify(ctx, cfg, payments.VerifyRequest{
		Reference: payment.ExternalReference, Data: payload, Charged: charged, Currency: payment.Currency,
	})
	if err != nil {
		return err
	}
	return s.apply(payment, v)
}

func (s *Service) apply(payment *db.OnlinePayment, v payments.Verification) error {
	switch v.Status {
	case payments.StatusPaid:
		currency := v.Currency
		if currency == "" {
			currency = payment.Currency
		}
		captured, err := s.store.CapturePayment(payment.AttemptID, v.Amount, currency, v.Receipt)
		if err != nil {
			return err
		}
		if payment.Status == db.PaymentStatusCaptured && payment.AttemptStatus != db.PaymentAttemptStatusCompleted {
			slog.Warn("duplicate payment captured; refund required", "payment_id", payment.ID, "attempt_id", payment.AttemptID)
		}
		if captured {
			slog.Info("payment captured", "payment_id", payment.ID, "order_id", payment.OrderID, "provider", payment.Provider)
			if payment.OrderStatus == db.OrderStatusCancelled {
				slog.Warn("payment captured after order cancellation; refund required", "payment_id", payment.ID, "order_id", payment.OrderID)
			}
		}
	case payments.StatusFailed:
		return s.store.FailPaymentAttempt(payment.AttemptID, v.Reason)
	}
	return nil
}

// HandleReturn confirms a payment when the customer's browser comes back
// from a hosted payment page. It returns the order to show.
func (s *Service) HandleReturn(ctx context.Context, userID int, reference string) (int, error) {
	payment, err := s.findByReference(reference)
	if err != nil {
		return 0, err
	}
	if payment.UserID != userID {
		return 0, db.ErrPaymentNotFound
	}
	payload, _ := s.store.PaymentPayload(payment.AttemptID)
	if payment.AttemptStatus != db.PaymentAttemptStatusCompleted {
		if err := s.verifyAndApply(ctx, payment, payload); err != nil {
			return payment.OrderID, err
		}
	}
	return payment.OrderID, nil
}

func (s *Service) findByReference(reference string) (*db.OnlinePayment, error) {
	reference = strings.TrimSpace(reference)
	for _, p := range s.registry.All() {
		if !p.Descriptor().Online {
			continue
		}
		if payment, err := s.store.GetPaymentByReference(p.Descriptor().ID, reference); err == nil {
			return payment, nil
		}
	}
	return nil, db.ErrPaymentNotFound
}

// HandleWebhook processes a signed provider notification (Paystack).
func (s *Service) HandleWebhook(ctx context.Context, providerID string, r *http.Request, body []byte) error {
	provider, cfg, err := s.providerFor(providerID)
	if err != nil {
		return err
	}
	parser, ok := provider.(payments.WebhookParser)
	if !ok {
		return payments.ErrUnsupported
	}
	event, err := parser.ParseWebhook(cfg, r, body)
	if err != nil {
		return err
	}
	payment, err := s.store.GetPaymentByReference(providerID, event.Reference)
	if err != nil {
		return err
	}
	if payment.AttemptStatus == db.PaymentAttemptStatusCompleted {
		return nil
	}
	payload, _ := s.store.PaymentPayload(payment.AttemptID)
	return s.verifyAndApply(ctx, payment, payload)
}

// HandleCallback processes an unsigned provider callback (M-Pesa). The
// per-payment token in the URL authenticates it, the provider identifiers
// must match the stored attempt, and a success is confirmed with the
// provider before capture.
func (s *Service) HandleCallback(ctx context.Context, providerID, token string, r *http.Request, body []byte) error {
	provider, cfg, err := s.providerFor(providerID)
	if err != nil {
		return err
	}
	parser, ok := provider.(payments.WebhookParser)
	if !ok {
		return payments.ErrUnsupported
	}
	payment, err := s.store.GetPaymentByCallbackToken(providerID, hashToken(token))
	if err != nil {
		return err
	}
	event, err := parser.ParseWebhook(cfg, r, body)
	if err != nil {
		return err
	}
	payload, _ := s.store.PaymentPayload(payment.AttemptID)
	if id := event.Data["checkout_request_id"]; id == "" || payload["checkout_request_id"] != "" && id != payload["checkout_request_id"] {
		return fmt.Errorf("%w: callback does not match the stored request", db.ErrPaymentNotFound)
	}
	if payload["checkout_request_id"] == "" {
		payload["checkout_request_id"] = event.Data["checkout_request_id"]
		if err := s.store.UpdatePaymentPayload(payment.AttemptID, map[string]string{"checkout_request_id": payload["checkout_request_id"]}); err != nil {
			return err
		}
	}
	if payment.AttemptStatus == db.PaymentAttemptStatusCompleted || event.Result == nil {
		return nil
	}
	if event.Result.Status == payments.StatusFailed {
		return s.store.FailPaymentAttempt(payment.AttemptID, event.Result.Reason)
	}
	charged := payment.Amount
	if c, err := strconv.ParseInt(payload["charged_cents"], 10, 64); err == nil && c > 0 {
		charged = money.Cents(c)
	}
	confirmed, err := provider.Verify(ctx, cfg, payments.VerifyRequest{Reference: payment.ExternalReference, Data: payload, Charged: charged, Currency: payment.Currency})
	if err != nil {
		return err
	}
	if confirmed.Status != payments.StatusPaid {
		return s.apply(payment, confirmed)
	}
	if event.Result.Amount > 0 {
		confirmed.Amount = event.Result.Amount
	}
	if event.Result.Receipt != "" {
		confirmed.Receipt = event.Result.Receipt
	}
	return s.apply(payment, confirmed)
}

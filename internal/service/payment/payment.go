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
	verifyInterval        = 5 * time.Second
	maxVerifiedReferences = 10_000
)

var (
	ErrInvalidMethod               = errors.New("invalid payment method")
	ErrForbidden                   = errors.New("forbidden payment settings change")
	ErrUnknownProvider             = errors.New("unknown payment provider")
	ErrMissingFields               = errors.New("required payment fields are missing")
	ErrCurrencyMismatch            = errors.New("provider does not support the store currency")
	ErrSecretsKeyMissing           = errors.New("SECRETS_KEY is required to store payment credentials")
	ErrLastMethod                  = errors.New("at least one payment method must stay enabled")
	ErrInvalidPricing              = errors.New("invalid pricing")
	ErrNotPayable                  = errors.New("this order cannot be paid online")
	ErrPaymentReviewRequired       = errors.New("payment amount discrepancy is under review")
	ErrNoPublicURL                 = errors.New("PUBLIC_BASE_URL is required for online payments")
	ErrProviderVerificationPending = errors.New("payment provider verification is pending")
)

type store interface {
	CreateAuditLog(ctx context.Context, actorUserID int, action, targetType string, targetID int, details string) error
	GetCommerceSettings(ctx context.Context) (db.CommerceSettings, error)
	UpdateCommerceSettings(context.Context, db.CommerceSettings) error
	UpdateCommerceSettingsForActor(ctx context.Context, actorID int, settings db.CommerceSettings, supportedProviders []string) error
	UpdateCommerceSettingsForActorIfUnchanged(ctx context.Context, actorID int, settings db.CommerceSettings, supportedProviders []string, expected db.CommerceSettings) error
	ListPaymentProviderConfigs(ctx context.Context) ([]db.PaymentProviderConfig, error)
	GetPaymentProviderConfig(ctx context.Context, provider string) (*db.PaymentProviderConfig, error)
	SavePaymentProviderConfig(context.Context, db.PaymentProviderConfig) error
	SavePaymentProviderConfigForActor(ctx context.Context, actorID int, config db.PaymentProviderConfig, supportedCurrencies []string) error
	GetOrderPayment(ctx context.Context, userID, orderID int) (*db.OnlinePayment, error)
	GetPaymentByReference(ctx context.Context, provider, reference string) (*db.OnlinePayment, error)
	GetPaymentByCallbackToken(ctx context.Context, provider, tokenHash string) (*db.OnlinePayment, error)
	StartPaymentAttempt(ctx context.Context, paymentID int, reference, callbackTokenHash string, payload map[string]string) (int, error)
	UpdatePaymentPayload(ctx context.Context, attemptID int, payload map[string]string) error
	PaymentPayload(ctx context.Context, attemptID int) (map[string]string, error)
	CapturePayment(ctx context.Context, attemptID int, confirmed money.Cents, currency, receipt string) (bool, error)
	FailPaymentAttempt(ctx context.Context, attemptID int, reason string) error
	LastPaymentError(ctx context.Context, paymentID int) (string, error)
	ListPaymentExceptions(ctx context.Context, limit int) ([]db.PaymentException, error)
	GetPaymentException(ctx context.Context, id int) (*db.PaymentException, error)
	BeginRefund(ctx context.Context, id int) (*db.PaymentException, bool, error)
	BeginRefundForActor(ctx context.Context, actorID, id int) (*db.PaymentException, bool, error)
	RecordMpesaRefundForActor(ctx context.Context, actorID, id int, receipt string) error
	SetPaymentExceptionStatus(ctx context.Context, id int, status, refundReference, lastError string) error
	SetPaymentExceptionStatusForActor(ctx context.Context, actorID, id int, status, refundReference, lastError string) error
	PaymentExceptionCounts(ctx context.Context) (open, pending, review int, err error)
}

var _ store = (*db.Store)(nil)

type paymentAmountMismatchRecorder interface {
	RecordPaymentAmountMismatch(context.Context, int, money.Cents, string, string) error
}

type paymentVerificationThrottle interface {
	TryAcquirePaymentVerification(context.Context, int, time.Duration) (bool, error)
}

// Actor is the admin making a settings change.
type Actor struct {
	ID   int
	Role string
}

type Service struct {
	store    store
	registry *payments.Registry
	box      *secret.Box

	mu             sync.Mutex
	verified       map[string]time.Time // hashed attempt reference -> last provider check
	saturatedUntil time.Time
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
func (s *Service) enabled(ctx context.Context) ([]enabledProvider, error) {
	configs, err := s.store.ListPaymentProviderConfigs(ctx)
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
func (s *Service) ValidateProduction(ctx context.Context, publicBaseURL string) error {
	settings, err := s.store.GetCommerceSettings(ctx)
	if err != nil {
		return err
	}
	configs, err := s.store.ListPaymentProviderConfigs(ctx)
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
func (s *Service) MethodOptions(ctx context.Context) []MethodOption {
	settings, err := s.store.GetCommerceSettings(ctx)
	if err != nil {
		slog.Error("load commerce settings", "error", err)
		return nil
	}
	providers, err := s.enabled(ctx)
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
func (s *Service) IsSupportedMethod(ctx context.Context, method, currency string) bool {
	method = strings.TrimSpace(strings.ToLower(method))
	if currency == "" {
		settings, err := s.store.GetCommerceSettings(ctx)
		if err != nil {
			return false
		}
		currency = settings.Currency
	}
	providers, err := s.enabled(ctx)
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
func (s *Service) BuildRecord(ctx context.Context, method string, totalAmount money.Cents, currency string) (db.PaymentRecordInput, error) {
	method = strings.TrimSpace(strings.ToLower(method))
	if !s.IsSupportedMethod(ctx, method, currency) {
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
	payment, err := s.store.GetOrderPayment(ctx, req.UserID, req.OrderID)
	if err != nil {
		return StartOutcome{}, err
	}
	if payment.ReviewRequired {
		return StartOutcome{}, ErrPaymentReviewRequired
	}
	if payment.Status != db.PaymentStatusPending || payment.OrderStatus == db.OrderStatusCancelled || !s.IsOnline(payment.Method) {
		return StartOutcome{}, ErrNotPayable
	}
	if strings.TrimSpace(req.BaseURL) == "" {
		return StartOutcome{}, ErrNoPublicURL
	}
	provider, cfg, err := s.providerFor(ctx, payment.Provider)
	if err != nil {
		return StartOutcome{}, err
	}
	if payment.AttemptStatus == db.PaymentStatusPending {
		payload, err := s.store.PaymentPayload(ctx, payment.AttemptID)
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
		shillings := payment.Amount.WholeUnitsCeil()
		if shillings < 1 {
			return StartOutcome{}, payments.ErrUnsupported
		}
		chargedCents, err := money.Cents(shillings).Mul(100)
		if err != nil {
			return StartOutcome{}, fmt.Errorf("%w: %w", payments.ErrUnsupported, err)
		}
		initial["charged_cents"] = strconv.FormatInt(int64(chargedCents), 10)
		initial["phone"] = req.Phone
	}
	attemptID, err := s.store.StartPaymentAttempt(ctx, payment.ID, reference, hashToken(token), initial)
	if errors.Is(err, db.ErrPaymentReviewRequired) {
		return StartOutcome{}, ErrPaymentReviewRequired
	}
	if errors.Is(err, db.ErrAttemptInProgress) {
		current, lookupErr := s.store.GetOrderPayment(ctx, req.UserID, req.OrderID)
		if lookupErr != nil {
			return StartOutcome{}, lookupErr
		}
		payload, lookupErr := s.store.PaymentPayload(ctx, current.AttemptID)
		if lookupErr != nil {
			return StartOutcome{}, lookupErr
		}
		return StartOutcome{RedirectURL: payload["redirect_url"]}, nil
	}
	if err != nil {
		return StartOutcome{}, err
	}
	// The attempt now exists; record the provider's answer even if the
	// customer's browser disconnects mid-request.
	ctx = context.WithoutCancel(ctx)
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
		if failErr := s.store.FailPaymentAttempt(ctx, attemptID, customerReason(err)); failErr != nil {
			slog.Error("record failed payment start", "attempt_id", attemptID, "error", failErr)
		}
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
	if err := s.store.UpdatePaymentPayload(ctx, attemptID, data); err != nil {
		slog.Error("persist payment start response", "attempt_id", attemptID, "error", err)
		// The request reference and callback token were stored before contacting
		// the provider, so keep the successful start usable while webhooks,
		// callbacks, or reference-based verification reconcile its result.
	}
	return StartOutcome{RedirectURL: result.RedirectURL}, nil
}

func (s *Service) providerFor(ctx context.Context, id string) (payments.Provider, payments.Config, error) {
	provider, ok := s.registry.Get(id)
	if !ok {
		return nil, payments.Config{}, ErrUnknownProvider
	}
	record, err := s.store.GetPaymentProviderConfig(ctx, id)
	if err != nil {
		return nil, payments.Config{}, err
	}
	if record == nil {
		return nil, payments.Config{}, payments.ErrNotConfigured
	}
	cfg, err := s.decrypt(*record)
	return provider, cfg, err
}

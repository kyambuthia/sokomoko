// Package payments defines payment providers and the registry of providers a
// store can enable. Each provider describes itself (name, countries,
// currencies, and the credentials it needs) so the admin workspace can render
// its setup form without provider-specific UI code.
//
// Adding a provider (for example Flutterwave, Stripe, or MTN MoMo):
//  1. Implement Provider (and WebhookParser if it pushes signed events).
//  2. Return its Descriptor with the fields an admin must fill in.
//  3. Register it in NewRegistry.
package payments

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/money"
)

var (
	ErrNotConfigured   = errors.New("payment provider is not configured")
	ErrUnsupported     = errors.New("payment provider does not support this request")
	ErrInvalidPhone    = errors.New("enter a valid mobile number")
	ErrProviderFailure = errors.New("payment provider request failed")
	ErrBadSignature    = errors.New("webhook signature is invalid")
)

// Field is one setting an admin fills in for a provider.
type Field struct {
	Key         string
	Label       string
	Help        string
	Secret      bool     // stored encrypted, never shown again
	Required    bool     // needed before the provider can be enabled
	Options     []string // renders a select when set
	Placeholder string
}

// Descriptor tells the admin workspace and checkout about a provider.
type Descriptor struct {
	ID         string
	Name       string
	Summary    string   // one line shown on the setup card
	Countries  []string // ISO country codes where it works
	Currencies []string // ISO currency codes it can charge
	Online     bool     // false for cash on delivery
	NeedsPhone bool     // checkout must collect a mobile number
	Fields     []Field
	DocsURL    string
}

// SupportsCurrency reports whether the provider can charge in code.
func (d Descriptor) SupportsCurrency(code string) bool {
	return len(d.Currencies) == 0 || slices.Contains(d.Currencies, strings.ToUpper(code))
}

// Config is a provider's decrypted configuration.
type Config struct {
	Mode   string            // "test" or "live"
	Values map[string]string // settings and secrets by field key
}

func (c Config) Value(key string) string { return strings.TrimSpace(c.Values[key]) }

// Live reports whether the provider should use its production endpoints.
func (c Config) Live() bool { return c.Mode == "live" }

// StartRequest asks a provider to collect money for one payment.
type StartRequest struct {
	PaymentID   int
	Reference   string // unique per attempt
	Amount      money.Cents
	Currency    string
	Email       string
	Phone       string // normalised by the provider when NeedsPhone
	Description string
	ReturnURL   string // where the customer's browser comes back
	CallbackURL string // where the provider calls the server
}

// StartResult says what happens next.
type StartResult struct {
	RedirectURL string            // send the customer here (hosted payment page)
	Data        map[string]string // identifiers to store with the payment
	Charged     money.Cents       // amount actually requested, if rounded
}

// Status is a provider's view of a payment.
type Status string

const (
	StatusPaid    Status = "paid"
	StatusPending Status = "pending"
	StatusFailed  Status = "failed"
)

// Verification is the authoritative result of asking the provider.
type Verification struct {
	Status   Status
	Amount   money.Cents // confirmed amount in minor units; 0 when unknown
	Currency string
	Receipt  string
	Reason   string // why it failed, safe to show the customer
}

// VerifyRequest identifies a payment to look up.
type VerifyRequest struct {
	Reference string
	Data      map[string]string // what Start returned
	Charged   money.Cents
	Currency  string
}

// Provider collects online payments.
type Provider interface {
	Descriptor() Descriptor
	Start(ctx context.Context, cfg Config, req StartRequest) (StartResult, error)
	Verify(ctx context.Context, cfg Config, req VerifyRequest) (Verification, error)
}

// WebhookEvent is what a provider's server-to-server notification refers to.
type WebhookEvent struct {
	Reference string            // the payment reference, when the provider sends it
	Data      map[string]string // provider identifiers, when it sends those instead
	Result    *Verification     // set when the notification itself carries the result
}

// WebhookParser authenticates and parses provider notifications.
type WebhookParser interface {
	ParseWebhook(cfg Config, r *http.Request, body []byte) (WebhookEvent, error)
}

// RefundProvider can initiate and check a refund for a confirmed charge.
type RefundProvider interface {
	CreateRefund(ctx context.Context, cfg Config, reference string, amount money.Cents, currency string) (RefundResult, error)
	FetchRefund(ctx context.Context, cfg Config, refundID string) (RefundResult, error)
}

type RefundResult struct {
	ID       string
	Status   string // provider status: pending, processing, needs-attention, failed, processed
	Amount   money.Cents
	Currency string
}

// Registry holds the providers a store can enable, in display order.
type Registry struct {
	providers []Provider
	byID      map[string]Provider
}

func newRegistry(providers ...Provider) *Registry {
	r := &Registry{byID: map[string]Provider{}}
	for _, p := range providers {
		r.providers = append(r.providers, p)
		r.byID[p.Descriptor().ID] = p
	}
	return r
}

// Options lets tests point providers at fake servers.
type Options struct {
	HTTPClient  *http.Client
	PaystackURL string
	MpesaURL    string // overrides both sandbox and production hosts
}

// NewRegistry returns every supported provider.
func NewRegistry(opts Options) *Registry {
	client := opts.HTTPClient
	if client == nil {
		client = defaultClient()
	}
	return newRegistry(
		CashOnDelivery{},
		&Mpesa{Client: client, BaseURL: opts.MpesaURL},
		&Paystack{Client: client, BaseURL: opts.PaystackURL},
	)
}

func (r *Registry) All() []Provider { return r.providers }

func (r *Registry) Get(id string) (Provider, bool) {
	p, ok := r.byID[id]
	return p, ok
}

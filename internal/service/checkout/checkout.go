// Package checkout prices carts, holds stock for open checkouts, and converts
// them into orders.
package checkout

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/money"
	paymentsvc "github.com/kyambuthia/sokomoko/internal/service/payment"
)

var (
	ErrCartEmpty            = errors.New("cart is empty")
	ErrDeliveryAddress      = errors.New("delivery address is required")
	ErrInvalidPaymentMethod = paymentsvc.ErrInvalidMethod
	ErrInsufficientStock    = errors.New("insufficient stock")
)

const (
	defaultPaymentMethod  = paymentsvc.MethodCashOnDelivery
	deliveryNoticePrefix  = "Order received. Awaiting partner acceptance."
	maxDeliveryAddressLen = 500

	// ReservationHoldTTL is how long stock stays held for an open checkout.
	ReservationHoldTTL = 15 * time.Minute
)

// Pricing is the store's money rules for a checkout.
type Pricing struct {
	Currency              string
	ShippingFlat          money.Cents
	FreeShippingThreshold money.Cents // 0 disables free shipping
	TaxRateBP             int64
}

// DefaultPricing matches the defaults seeded by migration 0003.
func DefaultPricing() Pricing {
	return Pricing{Currency: "USD", ShippingFlat: 650, FreeShippingThreshold: 8000, TaxRateBP: 800}
}

func pricingFrom(settings db.CommerceSettings) Pricing {
	return Pricing{
		Currency:              settings.Currency,
		ShippingFlat:          settings.ShippingFlat,
		FreeShippingThreshold: settings.FreeShippingThreshold,
		TaxRateBP:             settings.TaxRateBP,
	}
}

type paymentService interface {
	BuildRecord(method string, totalAmount money.Cents, currency string) (db.PaymentRecordInput, error)
	GenerateIdempotencyKey() (string, error)
	IsSupportedMethod(method, currency string) bool
}

type Summary struct {
	Currency    string
	Subtotal    money.Cents
	ShippingFee money.Cents
	TaxAmount   money.Cents
	Total       money.Cents
}

type Item struct {
	ProductID   int
	ProductName string
	Quantity    int
	UnitPrice   money.Cents
	LineTotal   money.Cents
}

type PageState struct {
	Token           string
	Currency        string
	Items           []Item
	Summary         Summary
	PaymentMethod   string
	DeliveryAddress string
	ExpiresAt       time.Time
	CanCheckout     bool
}

type Service struct {
	store    store
	payments paymentService
	now      func() time.Time
}

func New(store *db.Store, payments paymentService) *Service {
	return newWithStore(store, payments)
}

func newWithStore(store store, payments paymentService) *Service {
	return &Service{store: store, payments: payments, now: time.Now}
}

// CalculateSummary prices a subtotal: flat shipping below the free-shipping
// threshold plus estimated tax on the subtotal.
func CalculateSummary(subtotal money.Cents, pricing Pricing) Summary {
	shipping := pricing.ShippingFlat
	if subtotal == 0 || (pricing.FreeShippingThreshold > 0 && subtotal >= pricing.FreeShippingThreshold) {
		shipping = 0
	}
	tax := subtotal.MulBasisPoints(pricing.TaxRateBP)
	return Summary{
		Currency:    pricing.Currency,
		Subtotal:    subtotal,
		ShippingFee: shipping,
		TaxAmount:   tax,
		Total:       subtotal + shipping + tax,
	}
}

// PreparedCheckout returns the open checkout for checkoutToken, creating (and
// reserving stock for) one when the token is empty, unknown, or no longer open.
// With an empty token the user's latest open checkout is resumed.
func (s *Service) PreparedCheckout(userID int, checkoutToken string) (PageState, error) {
	key := strings.TrimSpace(checkoutToken)
	if key == "" {
		latest, err := s.store.GetLatestOpenCheckout(userID)
		if err != nil {
			return PageState{}, err
		}
		if s.isOpen(latest) {
			return mapPageState(latest), nil
		}
		if key, err = s.payments.GenerateIdempotencyKey(); err != nil {
			return PageState{}, err
		}
	}

	record, err := s.ensureOpen(userID, key)
	if err != nil {
		return PageState{Token: key}, err
	}
	return mapPageState(record), nil
}

// SaveDraft stores the delivery address and payment method on the checkout so
// a failed submission does not lose what the customer typed.
func (s *Service) SaveDraft(userID int, checkoutToken string, deliveryAddress string, paymentMethod string) (PageState, error) {
	key := strings.TrimSpace(checkoutToken)
	if key == "" {
		state, err := s.PreparedCheckout(userID, "")
		if err != nil {
			return state, err
		}
		key = state.Token
	}
	if paymentMethod != "" && !s.payments.IsSupportedMethod(paymentMethod, "") {
		paymentMethod = ""
	}
	address := truncate(strings.TrimSpace(deliveryAddress), maxDeliveryAddressLen)

	record, err := s.store.UpdateCheckoutDraft(userID, key, address, paymentMethod)
	if errors.Is(err, db.ErrCheckoutNotFound) {
		if _, err = s.ensureOpen(userID, key); err != nil {
			return PageState{Token: key}, err
		}
		record, err = s.store.UpdateCheckoutDraft(userID, key, address, paymentMethod)
	}
	if err != nil {
		return PageState{Token: key}, err
	}
	return mapPageState(record), nil
}

// Prepare reserves stock for the cart under reservationKey and snapshots the
// priced cart into an open checkout.
func (s *Service) Prepare(userID int, reservationKey string) error {
	for attempt := 0; attempt < 3; attempt++ {
		err := s.prepareOnce(userID, reservationKey)
		if !errors.Is(err, db.ErrCheckoutCurrencyChanged) {
			return err
		}
	}
	return ErrCartEmpty
}

func (s *Service) prepareOnce(userID int, reservationKey string) error {
	key := strings.TrimSpace(reservationKey)
	if key == "" {
		return nil
	}
	expiresAt := s.now().UTC().Add(ReservationHoldTTL)

	if err := s.store.ReserveCartForCheckout(userID, key, expiresAt); err != nil {
		return mapStoreError(err)
	}

	items, subtotal, err := s.store.GetCartItemsForCheckout(context.Background(), userID, key)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return ErrCartEmpty
	}

	lines := make([]db.CheckoutLineInput, 0, len(items))
	for _, item := range items {
		if item.Quantity > item.StockQuantity {
			return ErrInsufficientStock
		}
		lines = append(lines, db.CheckoutLineInput{
			ProductID:   item.ProductID,
			ProductName: item.ProductName,
			Quantity:    item.Quantity,
			UnitPrice:   item.UnitPrice,
		})
	}

	settings, err := s.store.GetCommerceSettings()
	if err != nil {
		return err
	}
	pricing := pricingFrom(settings)
	summary := CalculateSummary(subtotal, pricing)
	_, err = s.store.UpsertCheckout(userID, db.CheckoutInput{
		Token:          key,
		Currency:       pricing.Currency,
		PaymentMethod:  defaultPaymentMethod,
		SubtotalAmount: summary.Subtotal,
		ShippingFee:    summary.ShippingFee,
		TaxAmount:      summary.TaxAmount,
		TotalAmount:    summary.Total,
		ExpiresAt:      expiresAt,
		Lines:          lines,
	})
	if errors.Is(err, db.ErrCheckoutCurrencyChanged) {
		return err
	}
	return mapStoreError(err)
}

// CheckoutWithPayment places the order for the checkout identified by
// idempotencyKey. Replaying a key that already produced an order returns that
// order instead of creating another.
func (s *Service) CheckoutWithPayment(userID int, deliveryAddress, paymentMethod, idempotencyKey string) (int64, Summary, error) {
	address := strings.TrimSpace(deliveryAddress)
	if address == "" {
		return 0, Summary{}, ErrDeliveryAddress
	}
	address = truncate(address, maxDeliveryAddressLen)

	method := strings.TrimSpace(strings.ToLower(paymentMethod))
	if method == "" {
		method = defaultPaymentMethod
	}
	if !s.payments.IsSupportedMethod(method, "") {
		return 0, Summary{}, ErrInvalidPaymentMethod
	}

	key := strings.TrimSpace(idempotencyKey)
	if key != "" {
		existing, err := s.store.GetCheckoutPlacementByIdempotency(userID, key)
		if err != nil {
			return 0, Summary{}, err
		}
		if existing != nil {
			return existing.OrderID, summaryFromPlacement(*existing), nil
		}
	} else {
		generated, err := s.payments.GenerateIdempotencyKey()
		if err != nil {
			return 0, Summary{}, err
		}
		key = generated
	}

	record, err := s.ensureOpen(userID, key)
	if err != nil {
		return 0, Summary{}, err
	}

	summary := summaryFromCheckout(record)
	if !s.payments.IsSupportedMethod(method, record.Currency) {
		return 0, Summary{}, ErrInvalidPaymentMethod
	}
	paymentRecord, err := s.payments.BuildRecord(method, summary.Total, record.Currency)
	if err != nil {
		return 0, Summary{}, err
	}
	// The payment method and totals live on the payment and order rows; the
	// notice is only the customer-facing fulfillment message.
	notice := deliveryNoticePrefix

	placement, err := s.store.PlaceOrderFromCheckoutWithPayment(userID, key, address, notice, paymentRecord)
	if err != nil {
		return 0, Summary{}, mapStoreError(err)
	}
	if placement.Reused {
		summary = summaryFromPlacement(placement)
	}
	return placement.OrderID, summary, nil
}

// ensureOpen returns the open checkout for key, re-preparing it when it is
// missing, expired, or was invalidated by a cart change.
func (s *Service) ensureOpen(userID int, key string) (*db.Checkout, error) {
	record, err := s.store.GetCheckoutByToken(userID, key)
	if err != nil {
		return nil, err
	}
	if record != nil && record.Status == db.CheckoutStatusCompleted {
		return nil, ErrCartEmpty
	}
	if !s.isOpen(record) {
		if err := s.Prepare(userID, key); err != nil {
			return nil, err
		}
		if record, err = s.store.GetCheckoutByToken(userID, key); err != nil {
			return nil, err
		}
	}
	if record == nil || len(record.Lines) == 0 {
		return nil, ErrCartEmpty
	}
	return record, nil
}

func (s *Service) isOpen(checkout *db.Checkout) bool {
	return checkout != nil && checkout.Status == db.CheckoutStatusOpen && checkout.ExpiresAt.After(s.now())
}

func mapStoreError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, db.ErrCartEmpty), errors.Is(err, db.ErrCheckoutExpired), errors.Is(err, db.ErrCheckoutNotFound), errors.Is(err, db.ErrCheckoutCurrencyChanged):
		return ErrCartEmpty
	case errors.Is(err, db.ErrDeliveryAddressRequired):
		return ErrDeliveryAddress
	case errors.Is(err, db.ErrInsufficientStock):
		return ErrInsufficientStock
	default:
		return err
	}
}

func summaryFromCheckout(checkout *db.Checkout) Summary {
	if checkout == nil {
		return Summary{}
	}
	return Summary{
		Currency:    checkout.Currency,
		Subtotal:    checkout.SubtotalAmount,
		ShippingFee: checkout.ShippingFee,
		TaxAmount:   checkout.TaxAmount,
		Total:       checkout.TotalAmount,
	}
}

func summaryFromPlacement(placement db.CheckoutPlacement) Summary {
	return Summary{
		Currency: placement.Currency, Subtotal: placement.Subtotal,
		ShippingFee: placement.ShippingFee, TaxAmount: placement.TaxAmount,
		Total: placement.TotalAmount,
	}
}

func mapPageState(checkout *db.Checkout) PageState {
	if checkout == nil {
		return PageState{}
	}
	items := make([]Item, 0, len(checkout.Lines))
	for _, line := range checkout.Lines {
		items = append(items, Item{
			ProductID:   line.ProductID,
			ProductName: line.ProductName,
			Quantity:    line.Quantity,
			UnitPrice:   line.UnitPrice,
			LineTotal:   line.LineTotal,
		})
	}
	return PageState{
		Token:           checkout.Token,
		Currency:        checkout.Currency,
		Items:           items,
		Summary:         summaryFromCheckout(checkout),
		PaymentMethod:   checkout.PaymentMethod,
		DeliveryAddress: strings.TrimSpace(checkout.DeliveryAddress),
		ExpiresAt:       checkout.ExpiresAt,
		CanCheckout:     len(items) > 0,
	}
}

func truncate(value string, max int) string {
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
}

// Package checkout prices carts, holds stock for open checkouts, and converts
// them into orders.
package checkout

import (
	"errors"
	"fmt"
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
	shippingFeeFlat       money.Cents = 650
	freeShippingThreshold money.Cents = 8000
	estimatedTaxRateBP                = 800 // 8%
	currency                          = "USD"
	defaultPaymentMethod              = paymentsvc.MethodCashOnDelivery
	deliveryNoticePrefix              = "Order received. Awaiting partner acceptance."
	maxDeliveryAddressLen             = 500

	// ReservationHoldTTL is how long stock stays held for an open checkout.
	ReservationHoldTTL = 15 * time.Minute
)

type paymentService interface {
	BuildRecord(method string, totalAmount money.Cents) (db.PaymentRecordInput, error)
	GenerateIdempotencyKey() (string, error)
	IsSupportedMethod(method string) bool
	MethodLabel(method string) string
}

type Summary struct {
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
	if payments == nil {
		payments = paymentsvc.New()
	}
	return &Service{store: store, payments: payments, now: time.Now}
}

// CalculateSummary prices a subtotal: flat shipping below the free-shipping
// threshold plus estimated tax on the subtotal.
func CalculateSummary(subtotal money.Cents) Summary {
	shipping := shippingFeeFlat
	if subtotal >= freeShippingThreshold || subtotal == 0 {
		shipping = 0
	}
	tax := subtotal.MulBasisPoints(estimatedTaxRateBP)
	return Summary{
		Subtotal:    subtotal,
		ShippingFee: shipping,
		TaxAmount:   tax,
		Total:       subtotal + shipping + tax,
	}
}

// Checkout places a cash-on-delivery order for the cart in one step.
func (s *Service) Checkout(userID int, deliveryAddress string) (int64, error) {
	orderID, _, err := s.CheckoutWithPayment(userID, deliveryAddress, defaultPaymentMethod, "")
	return orderID, err
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
	if !s.payments.IsSupportedMethod(paymentMethod) {
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
	key := strings.TrimSpace(reservationKey)
	if key == "" {
		return nil
	}
	expiresAt := s.now().UTC().Add(ReservationHoldTTL)

	if err := s.store.ReserveCartForCheckout(userID, key, expiresAt); err != nil {
		return mapStoreError(err)
	}

	items, subtotal, err := s.store.GetCartItemsForCheckout(userID, key)
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

	summary := CalculateSummary(subtotal)
	_, err = s.store.UpsertCheckout(userID, db.CheckoutInput{
		Token:          key,
		Currency:       currency,
		PaymentMethod:  defaultPaymentMethod,
		SubtotalAmount: summary.Subtotal,
		ShippingFee:    summary.ShippingFee,
		TaxAmount:      summary.TaxAmount,
		TotalAmount:    summary.Total,
		ExpiresAt:      expiresAt,
		Lines:          lines,
	})
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
	if !s.payments.IsSupportedMethod(method) {
		return 0, Summary{}, ErrInvalidPaymentMethod
	}

	key := strings.TrimSpace(idempotencyKey)
	if key != "" {
		existing, err := s.store.GetCheckoutPlacementByIdempotency(userID, key)
		if err != nil {
			return 0, Summary{}, err
		}
		if existing != nil {
			return existing.OrderID, Summary{Total: existing.TotalAmount}, nil
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
	paymentRecord, err := s.payments.BuildRecord(method, summary.Total)
	if err != nil {
		return 0, Summary{}, err
	}
	notice := fmt.Sprintf(
		"%s Payment method selected: %s. Subtotal $%s, shipping $%s, estimated tax $%s.",
		deliveryNoticePrefix,
		s.payments.MethodLabel(method),
		summary.Subtotal, summary.ShippingFee, summary.TaxAmount,
	)

	placement, err := s.store.PlaceOrderFromCheckoutWithPayment(userID, key, address, notice, paymentRecord)
	if err != nil {
		return 0, Summary{}, mapStoreError(err)
	}
	if placement.Reused {
		summary = Summary{Total: placement.TotalAmount}
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
	case errors.Is(err, db.ErrCartEmpty), errors.Is(err, db.ErrCheckoutExpired), errors.Is(err, db.ErrCheckoutNotFound):
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
		Subtotal:    checkout.SubtotalAmount,
		ShippingFee: checkout.ShippingFee,
		TaxAmount:   checkout.TaxAmount,
		Total:       checkout.TotalAmount,
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

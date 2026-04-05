package checkout

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/kyambuthia/sokomoko/internal/db"
	paymentsvc "github.com/kyambuthia/sokomoko/internal/service/payment"
)

var (
	ErrCartEmpty            = errors.New("cart is empty")
	ErrDeliveryAddress      = errors.New("delivery address is required")
	ErrInvalidPaymentMethod = paymentsvc.ErrInvalidMethod
	ErrInsufficientStock    = errors.New("insufficient stock")
)

const (
	shippingFeeFlat             = 6.50
	freeShippingThreshold       = 80.00
	defaultEstimatedTaxRate     = 0.08
	defaultPaymentMethod        = paymentsvc.MethodCashOnDelivery
	deliveryNoticeDefaultPrefix = "Order received. Awaiting partner acceptance."
	reservationHoldTTL          = 15 * time.Minute
)

type paymentService interface {
	BuildRecord(method string, totalAmount float64) (db.PaymentRecordInput, error)
	GenerateIdempotencyKey() (string, error)
	IsSupportedMethod(method string) bool
	MethodLabel(method string) string
}

type Summary struct {
	Subtotal    float64
	ShippingFee float64
	TaxAmount   float64
	Total       float64
}

type Item struct {
	ProductID   int
	ProductName string
	Quantity    int
	UnitPrice   float64
	LineTotal   float64
}

type PageState struct {
	Token           string
	Items           []Item
	Summary         Summary
	PaymentMethod   string
	DeliveryAddress string
	CanCheckout     bool
}

type Service struct {
	store    store
	payments paymentService
}

func New(store *db.Store, payments paymentService) *Service {
	if payments == nil {
		payments = paymentsvc.New()
	}
	return &Service{store: newDBStore(store), payments: payments}
}

func newWithStore(store store, payments paymentService) *Service {
	if payments == nil {
		payments = paymentsvc.New()
	}
	return &Service{store: store, payments: payments}
}

func (s *Service) Checkout(userID int, deliveryAddress string) (int64, error) {
	orderID, _, err := s.CheckoutWithPayment(userID, deliveryAddress, defaultPaymentMethod, "")
	return orderID, err
}

func (s *Service) PreparedCheckout(userID int, checkoutToken string) (PageState, error) {
	key := strings.TrimSpace(checkoutToken)
	if key == "" {
		generatedKey, err := s.payments.GenerateIdempotencyKey()
		if err != nil {
			return PageState{}, err
		}
		key = generatedKey
	}

	record, err := s.store.GetCheckoutByToken(userID, key)
	if err != nil {
		return PageState{}, err
	}
	if !checkoutIsOpen(record) {
		if err := s.Prepare(userID, key); err != nil {
			return PageState{Token: key}, err
		}
		record, err = s.store.GetCheckoutByToken(userID, key)
		if err != nil {
			return PageState{}, err
		}
	}
	if record == nil || len(record.Lines) == 0 {
		return PageState{Token: key}, ErrCartEmpty
	}

	state := mapPageState(record)
	state.Token = key
	return state, nil
}

func (s *Service) Prepare(userID int, reservationKey string) error {
	key := strings.TrimSpace(reservationKey)
	if key == "" {
		return nil
	}

	if err := s.store.ReserveCartForCheckout(userID, key, time.Now().UTC().Add(reservationHoldTTL)); err != nil {
		switch {
		case errors.Is(err, db.ErrCartEmpty):
			return ErrCartEmpty
		case errors.Is(err, db.ErrInsufficientStock):
			return ErrInsufficientStock
		default:
			return err
		}
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
			ProductID:      item.ProductID,
			ProductName:    item.ProductName,
			Quantity:       item.Quantity,
			UnitPrice:      item.UnitPrice,
			LineTotal:      item.LineTotal,
			ReservationKey: key,
		})
	}

	summary := CalculateSummary(subtotal)
	if _, err := s.store.UpsertCheckout(userID, db.CheckoutInput{
		Token:          key,
		Currency:       "USD",
		PaymentMethod:  defaultPaymentMethod,
		SubtotalAmount: summary.Subtotal,
		ShippingFee:    summary.ShippingFee,
		TaxAmount:      summary.TaxAmount,
		TotalAmount:    summary.Total,
		ExpiresAt:      time.Now().UTC().Add(reservationHoldTTL),
		Lines:          lines,
	}); err != nil {
		return err
	}
	return nil
}

func (s *Service) CheckoutWithPayment(userID int, deliveryAddress, paymentMethod, idempotencyKey string) (int64, Summary, error) {
	address := strings.TrimSpace(deliveryAddress)
	if address == "" {
		return 0, Summary{}, ErrDeliveryAddress
	}

	method := strings.TrimSpace(strings.ToLower(paymentMethod))
	if method == "" {
		method = defaultPaymentMethod
	}
	if !s.payments.IsSupportedMethod(method) {
		return 0, Summary{}, ErrInvalidPaymentMethod
	}

	if existing, err := s.store.GetCheckoutPlacementByIdempotency(userID, idempotencyKey); err != nil {
		return 0, Summary{}, err
	} else if existing != nil {
		return existing.OrderID, Summary{Total: existing.TotalAmount}, nil
	}

	key := strings.TrimSpace(idempotencyKey)
	if key == "" {
		generatedKey, err := s.payments.GenerateIdempotencyKey()
		if err != nil {
			return 0, Summary{}, err
		}
		key = generatedKey
	}

	checkoutRecord, err := s.store.GetCheckoutByToken(userID, key)
	if err != nil {
		return 0, Summary{}, err
	}
	if !checkoutIsOpen(checkoutRecord) {
		if err := s.Prepare(userID, key); err != nil {
			return 0, Summary{}, err
		}
		checkoutRecord, err = s.store.GetCheckoutByToken(userID, key)
		if err != nil {
			return 0, Summary{}, err
		}
	}
	if checkoutRecord == nil || len(checkoutRecord.Lines) == 0 {
		return 0, Summary{}, ErrCartEmpty
	}

	summary := summaryFromCheckout(checkoutRecord)
	paymentRecord, err := s.payments.BuildRecord(method, summary.Total)
	if err != nil {
		return 0, Summary{}, err
	}
	notice := fmt.Sprintf(
		"%s Payment method selected: %s (processing placeholder). Subtotal $%.2f, shipping $%.2f, estimated tax $%.2f.",
		deliveryNoticeDefaultPrefix,
		s.payments.MethodLabel(method),
		summary.Subtotal,
		summary.ShippingFee,
		summary.TaxAmount,
	)

	placement, err := s.store.PlaceOrderFromCheckoutWithPayment(userID, key, address, notice, paymentRecord)
	if err == nil {
		return placement.OrderID, summary, nil
	}

	switch {
	case errors.Is(err, db.ErrCartEmpty):
		return 0, Summary{}, ErrCartEmpty
	case errors.Is(err, db.ErrDeliveryAddressRequired):
		return 0, Summary{}, ErrDeliveryAddress
	case errors.Is(err, db.ErrCheckoutExpired):
		return 0, Summary{}, ErrCartEmpty
	case errors.Is(err, db.ErrInsufficientStock):
		return 0, Summary{}, ErrInsufficientStock
	default:
		return 0, Summary{}, err
	}
}

func CalculateSummary(subtotal float64) Summary {
	subtotal = roundMoney(subtotal)
	shipping := shippingFeeFlat
	if subtotal >= freeShippingThreshold {
		shipping = 0
	}
	tax := roundMoney(subtotal * defaultEstimatedTaxRate)
	total := roundMoney(subtotal + shipping + tax)
	return Summary{
		Subtotal:    subtotal,
		ShippingFee: shipping,
		TaxAmount:   tax,
		Total:       total,
	}
}

func checkoutIsOpen(checkout *db.Checkout) bool {
	if checkout == nil || checkout.Status != db.CheckoutStatusOpen {
		return false
	}
	return !checkout.ExpiresAt.Valid || checkout.ExpiresAt.Time.After(time.Now().UTC())
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

	deliveryAddress := ""
	if checkout.DeliveryAddress.Valid {
		deliveryAddress = strings.TrimSpace(checkout.DeliveryAddress.String)
	}

	return PageState{
		Token:           checkout.Token,
		Items:           items,
		Summary:         summaryFromCheckout(checkout),
		PaymentMethod:   checkout.PaymentMethod,
		DeliveryAddress: deliveryAddress,
		CanCheckout:     len(items) > 0,
	}
}

func roundMoney(value float64) float64 {
	return math.Round(value*100) / 100
}

package checkout

import (
	"errors"
	"fmt"
	"math"
	"strings"

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
)

type paymentService interface {
	BuildRecord(method string, totalAmount float64) (db.PaymentRecordInput, error)
	IsSupportedMethod(method string) bool
	MethodLabel(method string) string
}

type Summary struct {
	Subtotal    float64
	ShippingFee float64
	TaxAmount   float64
	Total       float64
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

	items, subtotal, err := s.store.GetCartItems(userID)
	if err != nil {
		return 0, Summary{}, err
	}
	if len(items) == 0 {
		return 0, Summary{}, ErrCartEmpty
	}
	for _, item := range items {
		if item.Quantity > item.StockQuantity {
			return 0, Summary{}, ErrInsufficientStock
		}
	}

	summary := CalculateSummary(subtotal)
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

	placement, err := s.store.PlaceOrderFromCartWithPricingAndPayment(userID, address, summary.Total, notice, paymentRecord, strings.TrimSpace(idempotencyKey))
	if err == nil {
		return placement.OrderID, summary, nil
	}

	switch {
	case errors.Is(err, db.ErrCartEmpty):
		return 0, Summary{}, ErrCartEmpty
	case errors.Is(err, db.ErrDeliveryAddressRequired):
		return 0, Summary{}, ErrDeliveryAddress
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

func roundMoney(value float64) float64 {
	return math.Round(value*100) / 100
}

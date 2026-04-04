package commerce

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/db"
	paymentsvc "github.com/kyambuthia/sokomoko/internal/service/payment"
)

var (
	ErrInvalidProduct       = errors.New("invalid product")
	ErrInvalidQuantity      = errors.New("invalid quantity")
	ErrProductNotFound      = errors.New("product not found")
	ErrOutOfStock           = errors.New("product is out of stock")
	ErrInsufficientStock    = errors.New("insufficient stock")
	ErrCartEmpty            = errors.New("cart is empty")
	ErrDeliveryAddress      = errors.New("delivery address is required")
	ErrInvalidPaymentMethod = paymentsvc.ErrInvalidMethod
)

const (
	shippingFeeFlat             = 6.50
	freeShippingThreshold       = 80.00
	defaultEstimatedTaxRate     = 0.08
	defaultPaymentMethod        = paymentsvc.MethodCashOnDelivery
	deliveryNoticeDefaultPrefix = "Order received. Awaiting partner acceptance."
)

type Service struct {
	store    store
	payments paymentService
}

type paymentService interface {
	BuildRecord(method string, totalAmount float64) (db.PaymentRecordInput, error)
	IsSupportedMethod(method string) bool
	MethodLabel(method string) string
}

type CheckoutSummary struct {
	Subtotal    float64
	ShippingFee float64
	TaxAmount   float64
	Total       float64
}

type CartItem struct {
	ProductID     int
	ProductName   string
	UnitPrice     float64
	Quantity      int
	StockQuantity int
	LineTotal     float64
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

func (s *Service) GetCart(userID int) ([]CartItem, float64, error) {
	return s.store.GetCartItems(userID)
}

func (s *Service) AddToCart(userID, productID, quantity int) error {
	if productID <= 0 {
		return ErrInvalidProduct
	}
	if quantity <= 0 {
		return ErrInvalidQuantity
	}

	product, err := s.store.GetProductByID(productID)
	if err != nil {
		return err
	}
	if product == nil {
		return ErrProductNotFound
	}
	if product.StockQuantity <= 0 {
		return ErrOutOfStock
	}

	items, _, err := s.store.GetCartItems(userID)
	if err != nil {
		return err
	}
	existingQty := 0
	for _, item := range items {
		if item.ProductID == productID {
			existingQty = item.Quantity
			break
		}
	}
	if existingQty+quantity > product.StockQuantity {
		return ErrInsufficientStock
	}

	return s.store.AddToCart(userID, productID, quantity)
}

func mapCartItems(items []db.CartItem) []CartItem {
	mapped := make([]CartItem, 0, len(items))
	for _, item := range items {
		mapped = append(mapped, CartItem{
			ProductID:     item.ProductID,
			ProductName:   item.ProductName,
			UnitPrice:     item.UnitPrice,
			Quantity:      item.Quantity,
			StockQuantity: item.StockQuantity,
			LineTotal:     item.LineTotal,
		})
	}
	return mapped
}

func (s *Service) UpdateCartItem(userID, productID, quantity int) error {
	if productID <= 0 {
		return ErrInvalidProduct
	}
	if quantity < 0 {
		return ErrInvalidQuantity
	}
	if quantity == 0 {
		return s.store.UpdateCartQuantity(userID, productID, quantity)
	}

	product, err := s.store.GetProductByID(productID)
	if err != nil {
		return err
	}
	if product == nil {
		return ErrProductNotFound
	}
	if product.StockQuantity <= 0 {
		return ErrOutOfStock
	}
	if quantity > product.StockQuantity {
		return ErrInsufficientStock
	}

	return s.store.UpdateCartQuantity(userID, productID, quantity)
}

func (s *Service) RemoveFromCart(userID, productID int) error {
	if productID <= 0 {
		return ErrInvalidProduct
	}
	return s.store.RemoveFromCart(userID, productID)
}

func (s *Service) Checkout(userID int, deliveryAddress string) (int64, error) {
	orderID, _, err := s.CheckoutWithPayment(userID, deliveryAddress, defaultPaymentMethod, "")
	return orderID, err
}

func (s *Service) CheckoutWithPayment(userID int, deliveryAddress, paymentMethod, idempotencyKey string) (int64, CheckoutSummary, error) {
	address := strings.TrimSpace(deliveryAddress)
	if address == "" {
		return 0, CheckoutSummary{}, ErrDeliveryAddress
	}

	method := strings.TrimSpace(strings.ToLower(paymentMethod))
	if method == "" {
		method = defaultPaymentMethod
	}
	if !s.payments.IsSupportedMethod(method) {
		return 0, CheckoutSummary{}, ErrInvalidPaymentMethod
	}

	if existing, err := s.store.GetCheckoutPlacementByIdempotency(userID, idempotencyKey); err != nil {
		return 0, CheckoutSummary{}, err
	} else if existing != nil {
		return existing.OrderID, CheckoutSummary{Total: existing.TotalAmount}, nil
	}

	items, subtotal, err := s.store.GetCartItems(userID)
	if err != nil {
		return 0, CheckoutSummary{}, err
	}
	if len(items) == 0 {
		return 0, CheckoutSummary{}, ErrCartEmpty
	}
	for _, item := range items {
		if item.Quantity > item.StockQuantity {
			return 0, CheckoutSummary{}, ErrInsufficientStock
		}
	}

	summary := CalculateCheckoutSummary(subtotal)
	paymentRecord, err := s.payments.BuildRecord(method, summary.Total)
	if err != nil {
		return 0, CheckoutSummary{}, err
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
		return 0, CheckoutSummary{}, ErrCartEmpty
	case errors.Is(err, db.ErrDeliveryAddressRequired):
		return 0, CheckoutSummary{}, ErrDeliveryAddress
	case errors.Is(err, db.ErrInsufficientStock):
		return 0, CheckoutSummary{}, ErrInsufficientStock
	default:
		return 0, CheckoutSummary{}, err
	}
}

func CalculateCheckoutSummary(subtotal float64) CheckoutSummary {
	subtotal = roundMoney(subtotal)
	shipping := shippingFeeFlat
	if subtotal >= freeShippingThreshold {
		shipping = 0
	}
	tax := roundMoney(subtotal * defaultEstimatedTaxRate)
	total := roundMoney(subtotal + shipping + tax)
	return CheckoutSummary{
		Subtotal:    subtotal,
		ShippingFee: shipping,
		TaxAmount:   tax,
		Total:       total,
	}
}

func SupportedPaymentMethods() []string {
	return paymentsvc.New().SupportedMethods()
}

func PaymentMethodLabel(method string) string {
	return paymentsvc.New().MethodLabel(method)
}

func roundMoney(value float64) float64 {
	return math.Round(value*100) / 100
}

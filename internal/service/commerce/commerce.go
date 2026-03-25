package commerce

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/db"
)

var (
	ErrInvalidProduct       = errors.New("invalid product")
	ErrInvalidQuantity      = errors.New("invalid quantity")
	ErrProductNotFound      = errors.New("product not found")
	ErrOutOfStock           = errors.New("product is out of stock")
	ErrInsufficientStock    = errors.New("insufficient stock")
	ErrCartEmpty            = errors.New("cart is empty")
	ErrDeliveryAddress      = errors.New("delivery address is required")
	ErrInvalidPaymentMethod = errors.New("invalid payment method")
)

const (
	PaymentMethodCashOnDelivery    = "cash_on_delivery"
	PaymentMethodCardPlaceholder   = "card_placeholder"
	PaymentMethodMobilePlaceholder = "mobile_money_placeholder"
)

const (
	shippingFeeFlat             = 6.50
	freeShippingThreshold       = 80.00
	defaultEstimatedTaxRate     = 0.08
	defaultPaymentMethod        = PaymentMethodCashOnDelivery
	deliveryNoticeDefaultPrefix = "Order received. Awaiting partner acceptance."
)

type Service struct {
	store *db.Store
}

type CheckoutSummary struct {
	Subtotal    float64
	ShippingFee float64
	TaxAmount   float64
	Total       float64
}

func New(store *db.Store) *Service {
	return &Service{store: store}
}

func (s *Service) GetCart(userID int) ([]db.CartItem, float64, error) {
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
	orderID, _, err := s.CheckoutWithPayment(userID, deliveryAddress, defaultPaymentMethod)
	return orderID, err
}

func (s *Service) CheckoutWithPayment(userID int, deliveryAddress, paymentMethod string) (int64, CheckoutSummary, error) {
	address := strings.TrimSpace(deliveryAddress)
	if address == "" {
		return 0, CheckoutSummary{}, ErrDeliveryAddress
	}

	method := strings.TrimSpace(strings.ToLower(paymentMethod))
	if method == "" {
		method = defaultPaymentMethod
	}
	if !isSupportedPaymentMethod(method) {
		return 0, CheckoutSummary{}, ErrInvalidPaymentMethod
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
	notice := fmt.Sprintf(
		"%s Payment method selected: %s (processing placeholder). Subtotal $%.2f, shipping $%.2f, estimated tax $%.2f.",
		deliveryNoticeDefaultPrefix,
		paymentMethodLabel(method),
		summary.Subtotal,
		summary.ShippingFee,
		summary.TaxAmount,
	)

	orderID, err := s.store.PlaceOrderFromCartWithPricing(userID, address, summary.Total, notice)
	if err == nil {
		return orderID, summary, nil
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
	return []string{
		PaymentMethodCashOnDelivery,
		PaymentMethodCardPlaceholder,
		PaymentMethodMobilePlaceholder,
	}
}

func PaymentMethodLabel(method string) string {
	return paymentMethodLabel(strings.TrimSpace(strings.ToLower(method)))
}

func paymentMethodLabel(method string) string {
	switch method {
	case PaymentMethodCardPlaceholder:
		return "Card (placeholder)"
	case PaymentMethodMobilePlaceholder:
		return "Mobile Money (placeholder)"
	default:
		return "Cash on Delivery"
	}
}

func isSupportedPaymentMethod(method string) bool {
	for _, candidate := range SupportedPaymentMethods() {
		if method == candidate {
			return true
		}
	}
	return false
}

func roundMoney(value float64) float64 {
	return math.Round(value*100) / 100
}

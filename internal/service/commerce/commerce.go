package commerce

import (
	"errors"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/db"
)

var (
	ErrInvalidProduct  = errors.New("invalid product")
	ErrInvalidQuantity = errors.New("invalid quantity")
	ErrProductNotFound = errors.New("product not found")
	ErrOutOfStock      = errors.New("product is out of stock")
	ErrCartEmpty       = errors.New("cart is empty")
	ErrDeliveryAddress = errors.New("delivery address is required")
)

type Service struct {
	store *db.Store
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
	return s.store.AddToCart(userID, productID, quantity)
}

func (s *Service) UpdateCartItem(userID, productID, quantity int) error {
	if productID <= 0 {
		return ErrInvalidProduct
	}
	if quantity < 0 {
		return ErrInvalidQuantity
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
	address := strings.TrimSpace(deliveryAddress)
	if address == "" {
		return 0, ErrDeliveryAddress
	}
	orderID, err := s.store.PlaceOrderFromCart(userID, address)
	if err == nil {
		return orderID, nil
	}
	errMsg := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(errMsg, "cart is empty"):
		return 0, ErrCartEmpty
	case strings.Contains(errMsg, "delivery address is required"):
		return 0, ErrDeliveryAddress
	default:
		return 0, err
	}
}

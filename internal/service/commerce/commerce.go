package commerce

import (
	"errors"

	"github.com/kyambuthia/sokomoko/internal/db"
)

var (
	ErrInvalidProduct    = errors.New("invalid product")
	ErrInvalidQuantity   = errors.New("invalid quantity")
	ErrProductNotFound   = errors.New("product not found")
	ErrOutOfStock        = errors.New("product is out of stock")
	ErrInsufficientStock = errors.New("insufficient stock")
)

type Service struct{ store store }

type CartItem struct {
	ProductID     int
	ProductName   string
	UnitPrice     float64
	Quantity      int
	StockQuantity int
	LineTotal     float64
}

func New(store *db.Store) *Service {
	return &Service{store: newDBStore(store)}
}

func newWithStore(store store) *Service {
	return &Service{store: store}
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

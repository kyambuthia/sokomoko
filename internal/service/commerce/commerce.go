// Package commerce manages shopping carts.
package commerce

import (
	"errors"

	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/money"
	"github.com/kyambuthia/sokomoko/internal/ui"
)

var (
	ErrInvalidProduct    = errors.New("invalid product")
	ErrInvalidQuantity   = errors.New("invalid quantity")
	ErrProductNotFound   = errors.New("product not found")
	ErrOutOfStock        = errors.New("product is out of stock")
	ErrInsufficientStock = errors.New("insufficient stock")
)

// MaxLineQuantity caps a single cart line.
const MaxLineQuantity = 99

type CartItem struct {
	ProductID     int
	ProductName   string
	ProductSlug   string
	ImageURL      string
	UnitPrice     money.Cents
	Quantity      int
	StockQuantity int
	LineTotal     money.Cents
}

// Cart is a priced view of a user's cart.
type Cart struct {
	Items     []CartItem
	Subtotal  money.Cents
	ItemCount int
}

type Service struct{ store store }

func New(store *db.Store) *Service {
	return &Service{store: store}
}

func newWithStore(store store) *Service {
	return &Service{store: store}
}

func (s *Service) GetCart(userID int) ([]CartItem, money.Cents, error) {
	cart, err := s.Cart(userID)
	return cart.Items, cart.Subtotal, err
}

func (s *Service) Cart(userID int) (Cart, error) {
	items, subtotal, err := s.store.GetCartItems(userID)
	if err != nil {
		return Cart{}, err
	}
	cart := Cart{Items: make([]CartItem, 0, len(items)), Subtotal: subtotal}
	for _, item := range items {
		cart.Items = append(cart.Items, CartItem{
			ProductID:     item.ProductID,
			ProductName:   item.ProductName,
			ProductSlug:   item.ProductSlug,
			ImageURL:      ui.ProductImageURL(item.ProductImageURL, item.ProductSlug, item.ProductName, ""),
			UnitPrice:     item.UnitPrice,
			Quantity:      item.Quantity,
			StockQuantity: item.StockQuantity,
			LineTotal:     item.LineTotal,
		})
		cart.ItemCount += item.Quantity
	}
	return cart, nil
}

// ItemCount returns the total quantity in the user's cart.
func (s *Service) ItemCount(userID int) (int, error) {
	return s.store.CartItemCount(userID)
}

func (s *Service) AddToCart(userID, productID, quantity int) error {
	if productID <= 0 {
		return ErrInvalidProduct
	}
	if quantity <= 0 || quantity > MaxLineQuantity {
		return ErrInvalidQuantity
	}
	existing, err := s.store.CartQuantity(userID, productID)
	if err != nil {
		return err
	}
	if err := s.checkStock(userID, productID, existing+quantity); err != nil {
		return err
	}
	if existing+quantity > MaxLineQuantity {
		return ErrInvalidQuantity
	}
	return s.store.AddToCart(userID, productID, quantity)
}

// UpdateCartItem sets the quantity of a line; zero removes it.
func (s *Service) UpdateCartItem(userID, productID, quantity int) error {
	if productID <= 0 {
		return ErrInvalidProduct
	}
	if quantity < 0 || quantity > MaxLineQuantity {
		return ErrInvalidQuantity
	}
	if quantity > 0 {
		if err := s.checkStock(userID, productID, quantity); err != nil {
			return err
		}
	}
	return s.store.UpdateCartQuantity(userID, productID, quantity)
}

func (s *Service) RemoveFromCart(userID, productID int) error {
	if productID <= 0 {
		return ErrInvalidProduct
	}
	return s.store.RemoveFromCart(userID, productID)
}

func (s *Service) checkStock(userID, productID, wanted int) error {
	available, err := s.store.AvailableForUser(userID, productID)
	if errors.Is(err, db.ErrProductNotFound) {
		return ErrProductNotFound
	}
	if err != nil {
		return err
	}
	if available <= 0 {
		return ErrOutOfStock
	}
	if wanted > available {
		return ErrInsufficientStock
	}
	return nil
}

package commerce

import (
	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/money"
)

// store is the subset of *db.Store the commerce service depends on.
type store interface {
	AddToCart(userID, productID, quantity int) error
	AvailableForUser(userID, productID int) (int, error)
	CartItemCount(userID int) (int, error)
	CartQuantity(userID, productID int) (int, error)
	GetCartItems(userID int) ([]db.CartItem, money.Cents, error)
	RemoveFromCart(userID, productID int) error
	UpdateCartQuantity(userID, productID, quantity int) error
}

var _ store = (*db.Store)(nil)

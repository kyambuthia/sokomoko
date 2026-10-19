package commerce

import (
	"context"

	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/money"
)

// store is the subset of *db.Store the commerce service depends on.
type store interface {
	AddToCart(ctx context.Context, userID, productID, quantity int) error
	AvailableForUser(ctx context.Context, userID, productID int) (int, error)
	CartItemCount(ctx context.Context, userID int) (int, error)
	CartQuantity(ctx context.Context, userID, productID int) (int, error)
	GetCartItems(ctx context.Context, userID int) ([]db.CartItem, money.Cents, error)
	RemoveFromCart(ctx context.Context, userID, productID int) error
	UpdateCartQuantity(ctx context.Context, userID, productID, quantity int) error
}

var _ store = (*db.Store)(nil)

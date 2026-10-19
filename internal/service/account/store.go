package account

import (
	"context"

	"github.com/kyambuthia/sokomoko/internal/db"
)

type store interface {
	GetOrderForUser(context.Context, int, int) (*db.Order, error)
	ListOrdersByUserPage(context.Context, int, int, int) ([]db.Order, int, error)
	CancelOrderByCustomer(ctx context.Context, userID, orderID int) error
}

var _ store = (*db.Store)(nil)

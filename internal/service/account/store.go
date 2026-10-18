package account

import (
	"context"

	"github.com/kyambuthia/sokomoko/internal/db"
)

type store interface {
	GetOrderForUser(userID, orderID int) (*db.Order, error)
	GetOrderForUserContext(context.Context, int, int) (*db.Order, error)
	ListOrdersByUser(userID int) ([]db.Order, error)
	ListOrdersByUserPageContext(context.Context, int, int, int) ([]db.Order, int, error)
}

var _ store = (*db.Store)(nil)

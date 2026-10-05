package account

import "github.com/kyambuthia/sokomoko/internal/db"

type store interface {
	GetOrderForUser(userID, orderID int) (*db.Order, error)
	ListOrdersByUser(userID int) ([]db.Order, error)
}

var _ store = (*db.Store)(nil)

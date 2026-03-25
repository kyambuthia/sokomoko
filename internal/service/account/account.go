package account

import "github.com/kyambuthia/sokomoko/internal/db"

type Service struct {
	store store
}

type store interface {
	ListOrdersByUser(userID int) ([]db.CustomerOrder, error)
}

func New(store store) *Service {
	return &Service{store: store}
}

func (s *Service) OrdersForUser(userID int) ([]db.CustomerOrder, error) {
	return s.store.ListOrdersByUser(userID)
}

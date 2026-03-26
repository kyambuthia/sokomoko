package account

import "github.com/kyambuthia/sokomoko/internal/db"

type Order struct {
	ID             int
	Status         string
	PartnerStatus  string
	DeliveryStatus string
	DeliveryNotice string
	TotalAmount    float64
}

type Service struct {
	store store
}

func New(store *db.Store) *Service {
	return &Service{store: newDBStore(store)}
}

func newWithStore(store store) *Service {
	return &Service{store: store}
}

func (s *Service) OrdersForUser(userID int) ([]Order, error) {
	return s.store.ListOrdersByUser(userID)
}

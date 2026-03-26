package account

import "github.com/kyambuthia/sokomoko/internal/db"

type store interface {
	ListOrdersByUser(userID int) ([]Order, error)
}

type dbStore struct {
	store *db.Store
}

func newDBStore(store *db.Store) store {
	return &dbStore{store: store}
}

func (s *dbStore) ListOrdersByUser(userID int) ([]Order, error) {
	orders, err := s.store.ListOrdersByUser(userID)
	if err != nil {
		return nil, err
	}

	mapped := make([]Order, 0, len(orders))
	for _, order := range orders {
		mapped = append(mapped, Order{
			ID:             order.ID,
			Status:         order.Status,
			PartnerStatus:  order.PartnerStatus,
			DeliveryStatus: order.DeliveryStatus,
			DeliveryNotice: order.DeliveryNotice,
			TotalAmount:    order.TotalAmount,
		})
	}
	return mapped, nil
}

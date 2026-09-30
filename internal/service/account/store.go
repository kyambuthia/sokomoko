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
		items := make([]OrderItem, 0, len(order.Items))
		for _, item := range order.Items {
			items = append(items, OrderItem{
				ProductID:   item.ProductID,
				ProductName: item.ProductName,
				Quantity:    item.Quantity,
				UnitPrice:   item.UnitPrice,
				LineTotal:   item.LineTotal,
			})
		}
		mapped = append(mapped, Order{
			ID:              order.ID,
			Status:          order.Status,
			PartnerStatus:   order.PartnerStatus,
			DeliveryStatus:  order.DeliveryStatus,
			DeliveryNotice:  order.DeliveryNotice,
			DeliveryAddress: order.DeliveryAddress,
			TotalAmount:     order.TotalAmount,
			CreatedAt:       order.CreatedAt,
			Items:           items,
		})
	}
	return mapped, nil
}

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

type store interface {
	ListOrdersByUser(userID int) ([]db.CustomerOrder, error)
}

func New(store store) *Service {
	return &Service{store: store}
}

func (s *Service) OrdersForUser(userID int) ([]Order, error) {
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

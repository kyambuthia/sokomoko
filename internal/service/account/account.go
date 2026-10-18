// Package account serves a customer's own order history.
package account

import (
	"time"

	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/money"
)

type OrderItem struct {
	ProductID   int
	ProductName string
	Quantity    int
	UnitPrice   money.Cents
	LineTotal   money.Cents
}

type Order struct {
	ID              int
	Currency        string
	Status          string
	PartnerStatus   string
	DeliveryStatus  string
	DeliveryNotice  string
	DeliveryAddress string
	Subtotal        money.Cents
	ShippingFee     money.Cents
	TaxAmount       money.Cents
	TotalAmount     money.Cents
	CreatedAt       time.Time
	UpdatedAt       time.Time
	PaymentMethod   string
	PaymentStatus   string
	AwaitingPayment bool // online payment still outstanding
	Items           []OrderItem
}

type Service struct {
	store store
}

func New(store *db.Store) *Service {
	return &Service{store: store}
}

func newWithStore(store store) *Service {
	return &Service{store: store}
}

func (s *Service) OrdersForUser(userID int) ([]Order, error) {
	orders, err := s.store.ListOrdersByUser(userID)
	if err != nil {
		return nil, err
	}
	mapped := make([]Order, 0, len(orders))
	for _, order := range orders {
		mapped = append(mapped, mapOrder(order))
	}
	return mapped, nil
}

// OrderForUser returns one of the user's orders, or nil when it is not theirs.
func (s *Service) OrderForUser(userID, orderID int) (*Order, error) {
	order, err := s.store.GetOrderForUser(userID, orderID)
	if err != nil || order == nil {
		return nil, err
	}
	mapped := mapOrder(*order)
	return &mapped, nil
}

func mapOrder(order db.Order) Order {
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
	return Order{
		ID:              order.ID,
		Currency:        order.Currency,
		Status:          order.Status,
		PartnerStatus:   order.PartnerStatus,
		DeliveryStatus:  order.DeliveryStatus,
		DeliveryNotice:  order.DeliveryNotice,
		DeliveryAddress: order.DeliveryAddress,
		Subtotal:        order.Subtotal,
		ShippingFee:     order.ShippingFee,
		TaxAmount:       order.TaxAmount,
		TotalAmount:     order.TotalAmount,
		CreatedAt:       order.CreatedAt,
		UpdatedAt:       order.UpdatedAt,
		PaymentMethod:   order.PaymentMethod,
		PaymentStatus:   order.PaymentStatus,
		AwaitingPayment: order.AwaitingPayment(),
		Items:           items,
	}
}

// Package account serves a customer's own order history.
package account

import (
	"context"
	"errors"
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
	Cancellable     bool // the customer may still cancel it
	Items           []OrderItem
}

var (
	ErrOrderNotFound       = errors.New("order not found")
	ErrOrderNotCancellable = errors.New("order can no longer be cancelled")
)

// CancelOrder cancels one of the user's orders before the vendor accepts it
// and before any payment is captured.
func (s *Service) CancelOrder(ctx context.Context, userID, orderID int) error {
	if userID <= 0 || orderID <= 0 {
		return ErrOrderNotFound
	}
	err := s.store.CancelOrderByCustomer(ctx, userID, orderID)
	switch {
	case errors.Is(err, db.ErrOrderNotFound):
		return ErrOrderNotFound
	case errors.Is(err, db.ErrOrderNotCancellable):
		return ErrOrderNotCancellable
	}
	return err
}

type Service struct {
	store store
}

func New(store *db.Store) *Service {
	return &Service{store: store}
}

func (s *Service) OrdersForUserPage(ctx context.Context, userID, page, limit int) ([]Order, int, error) {
	orders, total, err := s.store.ListOrdersByUserPage(ctx, userID, limit, (page-1)*limit)
	if err != nil {
		return nil, 0, err
	}
	mapped := make([]Order, 0, len(orders))
	for _, order := range orders {
		mapped = append(mapped, mapOrder(order))
	}
	return mapped, total, nil
}

// OrderForUser returns one of the user's orders, or nil when it is not theirs.
func (s *Service) OrderForUser(ctx context.Context, userID, orderID int) (*Order, error) {
	order, err := s.store.GetOrderForUser(ctx, userID, orderID)
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
		Cancellable: order.Status == db.OrderStatusPending && order.PartnerStatus == "new" &&
			order.PaymentStatus != db.PaymentStatusCaptured && order.PaymentStatus != db.PaymentStatusRefunded,
		Items: items,
	}
}

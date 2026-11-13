package partner

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/money"
)

const maxNoticeLength = 500

type Summary struct {
	NewCount        int
	InProgressCount int
	DispatchedCount int
	CompletedCount  int
	OverdueCount    int
}

type OrderItem struct {
	ProductID   int
	ProductName string
	Quantity    int
	LineTotal   money.Cents
}

type Order struct {
	ID              int
	Currency        string
	CustomerName    string
	Status          string
	PartnerStatus   string
	DeliveryStatus  string
	DeliveryNotice  string
	DeliveryAddress string
	TotalAmount     money.Cents
	CreatedAt       time.Time
	Items           []OrderItem
}

type OrdersData struct {
	Filter  string
	Orders  []Order
	Summary Summary
}

type UpdateOrderInput struct {
	OrderID        string
	PartnerStatus  string
	DeliveryStatus string
	DeliveryNotice string
}

func (s *Service) OrdersPageForActor(ctx context.Context, actor *Actor, filter string, page, limit int) (OrdersData, int, error) {
	clean := strings.TrimSpace(filter)
	if clean == "" {
		clean = "all"
	}
	partnerID, err := s.partnerIDForActor(ctx, actor)
	if err != nil {
		return OrdersData{}, 0, err
	}
	if actorIsPartner(actor) && partnerID == 0 {
		return OrdersData{Filter: clean}, 0, nil
	}
	orders, summary, total, err := s.store.ListFulfillmentOrdersPageWithSummary(ctx, partnerID, clean, limit, (page-1)*limit)
	if err != nil {
		return OrdersData{}, 0, err
	}
	mapped := make([]Order, 0, len(orders))
	for _, order := range orders {
		mapped = append(mapped, mapOrder(order))
	}
	return OrdersData{Filter: clean, Orders: mapped, Summary: Summary(summary)}, total, nil
}

func (s *Service) OrdersForActor(ctx context.Context, actor *Actor, filter string) (OrdersData, error) {
	if !canUseWorkspace(actor) {
		return OrdersData{}, ErrForbiddenWorkspace
	}
	cleanFilter := strings.TrimSpace(filter)
	if cleanFilter == "" {
		cleanFilter = "all"
	}
	var orders []db.Order
	if actorIsPartner(actor) {
		partnerID, err := s.store.PartnerIDForUser(ctx, actor.ID)
		if err != nil {
			return OrdersData{}, err
		}
		if orders, err = s.store.ListOrdersForPartner(ctx, partnerID); err != nil {
			return OrdersData{}, err
		}
	} else {
		var err error
		if orders, err = s.store.ListOrdersForFulfillment(ctx); err != nil {
			return OrdersData{}, err
		}
	}
	filtered := make([]Order, 0, len(orders))
	for _, order := range orders {
		if matchesOrderFilter(order, cleanFilter) {
			filtered = append(filtered, mapOrder(order))
		}
	}
	if actorIsPartner(actor) {
		mapped := make([]Order, 0, len(orders))
		for _, order := range orders {
			mapped = append(mapped, mapOrder(order))
		}
		return OrdersData{Filter: cleanFilter, Orders: filtered, Summary: summaryForOrders(mapped)}, nil
	}
	summary, err := s.store.GetPartnerOrderSummary(ctx)
	if err != nil {
		return OrdersData{}, err
	}
	return OrdersData{Filter: cleanFilter, Orders: filtered, Summary: Summary(summary)}, nil
}

func (s *Service) UpdateOrder(ctx context.Context, actor *Actor, input UpdateOrderInput) error {
	if !canUseWorkspace(actor) {
		return ErrForbiddenOrderUpdate
	}
	orderID, err := strconv.Atoi(strings.TrimSpace(input.OrderID))
	if err != nil || orderID <= 0 {
		return ErrInvalidOrderID
	}
	partnerStatus := strings.TrimSpace(input.PartnerStatus)
	deliveryStatus := strings.TrimSpace(input.DeliveryStatus)
	notice := strings.TrimSpace(input.DeliveryNotice)
	if runes := []rune(notice); len(runes) > maxNoticeLength {
		notice = string(runes[:maxNoticeLength])
	}

	if err := s.store.UpdateOrderFulfillmentForActor(ctx, actor.ID, actor.Role, orderID, partnerStatus, deliveryStatus, notice); err != nil {
		switch {
		case errors.Is(err, db.ErrOrderNotFound):
			if actorIsPartner(actor) {
				return ErrForbiddenOrderUpdate
			}
			return ErrOrderNotFound
		case errors.Is(err, db.ErrInvalidPartnerTransition),
			errors.Is(err, db.ErrInvalidDeliveryTransition),
			errors.Is(err, db.ErrInvalidPartnerStatus),
			errors.Is(err, db.ErrInvalidDeliveryStatus),
			errors.Is(err, db.ErrInsufficientStock):
			return ErrInvalidOrderTransition
		default:
			return err
		}
	}
	s.logAudit(ctx, actor.ID, "partner.fulfillment.update", "order", orderID,
		"partner="+partnerStatus+",delivery="+deliveryStatus)
	return nil
}

func mapOrder(order db.Order) Order {
	items := make([]OrderItem, 0, len(order.Items))
	for _, item := range order.Items {
		items = append(items, OrderItem{ProductID: item.ProductID, ProductName: item.ProductName, Quantity: item.Quantity, LineTotal: item.LineTotal})
	}
	return Order{
		ID:              order.ID,
		Currency:        order.Currency,
		CustomerName:    order.CustomerName,
		Status:          order.Status,
		PartnerStatus:   order.PartnerStatus,
		DeliveryStatus:  order.DeliveryStatus,
		DeliveryNotice:  order.DeliveryNotice,
		DeliveryAddress: order.DeliveryAddress,
		TotalAmount:     order.TotalAmount,
		CreatedAt:       order.CreatedAt,
		Items:           items,
	}
}

func matchesOrderFilter(order db.Order, filter string) bool {
	switch filter {
	case "all":
		return true
	case "completed":
		return order.Status == db.OrderStatusDelivered
	case "dispatched":
		return order.PartnerStatus == "dispatched" && order.Status != db.OrderStatusDelivered
	default:
		return order.PartnerStatus == filter
	}
}

func summaryForOrders(orders []Order) Summary {
	var summary Summary
	for _, order := range orders {
		if order.Status == "cancelled" {
			continue
		}
		if order.Status == db.OrderStatusDelivered {
			summary.CompletedCount++
			continue
		}
		switch order.PartnerStatus {
		case "new":
			summary.NewCount++
		case "accepted", "packing":
			summary.InProgressCount++
		case "dispatched":
			summary.DispatchedCount++
		}
		if (order.PartnerStatus == "new" || order.PartnerStatus == "accepted") &&
			!order.CreatedAt.IsZero() && time.Since(order.CreatedAt) >= 2*time.Hour {
			summary.OverdueCount++
		}
	}
	return summary
}

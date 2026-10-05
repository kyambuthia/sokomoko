// Package admin backs the platform administration workspace.
package admin

import (
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/money"
)

var (
	ErrForbiddenOrderUpdate = errors.New("forbidden order update")
	ErrForbiddenUserAction  = errors.New("forbidden user action")
	ErrInvalidOrderID       = errors.New("invalid order id")
	ErrOrderNotFound        = errors.New("order not found")
	ErrInvalidOrderState    = errors.New("invalid order state")
	ErrInvalidUserID        = errors.New("invalid user id")
	ErrUserNotFound         = errors.New("user not found")
	ErrProtectedUser        = errors.New("protected user")
)

type Service struct {
	store store
}

type Metrics struct {
	ProductCount int
	SessionCount int
	AdminCount   int
	StaffCount   int
	UserCount    int
}

type SalesReport struct {
	RevenueTotal   money.Cents
	OrderCount     int
	PendingCount   int
	ShippedCount   int
	DeliveredCount int
	CancelledCount int
}

type Product struct {
	ID            int
	Name          string
	Slug          string
	Category      string
	PartnerName   string
	Price         money.Cents
	StockQuantity int
}

type OrderItem struct {
	ProductName string
	Quantity    int
	LineTotal   money.Cents
}

type Order struct {
	ID              int
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

type TeamMember struct {
	ID        int
	Username  string
	Email     string
	Role      string
	CreatedAt time.Time
}

type AuditLog struct {
	CreatedAt   time.Time
	ActorUserID sql.NullInt64
	ActorName   string
	Action      string
	TargetType  string
	TargetID    sql.NullInt64
	Details     string
}

type Actor struct {
	ID   int
	Role string
}

type UpdateOrderInput struct {
	OrderID        string
	Status         string
	PartnerStatus  string
	DeliveryStatus string
	DeliveryNotice string
}

func New(store *db.Store) *Service {
	return &Service{store: store}
}

func newWithStore(store store) *Service {
	return &Service{store: store}
}

func (s *Service) Metrics() (Metrics, error) {
	var (
		m   Metrics
		err error
	)
	if m.ProductCount, err = s.store.CountProducts(); err != nil {
		return Metrics{}, err
	}
	if m.SessionCount, err = s.store.CountActiveSessions(); err != nil {
		return Metrics{}, err
	}
	if m.AdminCount, err = s.store.CountUsersByRole(db.RoleAdmin); err != nil {
		return Metrics{}, err
	}
	if m.StaffCount, err = s.store.CountUsersByRole(db.RoleStaff); err != nil {
		return Metrics{}, err
	}
	if m.UserCount, err = s.store.CountUsersByRole(db.RoleUser); err != nil {
		return Metrics{}, err
	}
	return m, nil
}

func (s *Service) Products() ([]Product, error) {
	products, err := s.store.GetAllProducts()
	if err != nil {
		return nil, err
	}
	mapped := make([]Product, 0, len(products))
	for _, p := range products {
		mapped = append(mapped, Product{
			ID: p.ID, Name: p.Name, Slug: p.Slug, Category: p.Category, PartnerName: p.PartnerName,
			Price: p.Price, StockQuantity: p.StockQuantity,
		})
	}
	return mapped, nil
}

func (s *Service) Orders() ([]Order, error) {
	orders, err := s.store.ListAllOrders()
	if err != nil {
		return nil, err
	}
	mapped := make([]Order, 0, len(orders))
	for _, order := range orders {
		items := make([]OrderItem, 0, len(order.Items))
		for _, item := range order.Items {
			items = append(items, OrderItem{ProductName: item.ProductName, Quantity: item.Quantity, LineTotal: item.LineTotal})
		}
		mapped = append(mapped, Order{
			ID:              order.ID,
			CustomerName:    order.CustomerName,
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

func (s *Service) Reports() (SalesReport, error) {
	revenue, err := s.store.SumOrderRevenue()
	if err != nil {
		return SalesReport{}, err
	}
	counts, err := s.store.GetOrderStatusCounts()
	if err != nil {
		return SalesReport{}, err
	}
	report := SalesReport{
		RevenueTotal:   revenue,
		PendingCount:   counts[db.OrderStatusPending] + counts[db.OrderStatusProcessing],
		ShippedCount:   counts[db.OrderStatusShipped],
		DeliveredCount: counts[db.OrderStatusDelivered],
		CancelledCount: counts[db.OrderStatusCancelled],
	}
	for _, count := range counts {
		report.OrderCount += count
	}
	return report, nil
}

func (s *Service) UpdateOrder(actor *Actor, input UpdateOrderInput) error {
	if actor == nil || actor.Role != db.RoleAdmin {
		return ErrForbiddenOrderUpdate
	}
	orderID, err := parsePositiveInt(input.OrderID)
	if err != nil {
		return ErrInvalidOrderID
	}
	status := strings.TrimSpace(input.Status)
	partnerStatus := strings.TrimSpace(input.PartnerStatus)
	deliveryStatus := strings.TrimSpace(input.DeliveryStatus)
	notice := strings.TrimSpace(input.DeliveryNotice)

	if err := s.store.UpdateOrderByAdmin(orderID, status, partnerStatus, deliveryStatus, notice); err != nil {
		switch {
		case errors.Is(err, db.ErrOrderNotFound):
			return ErrOrderNotFound
		case errors.Is(err, db.ErrInvalidOrderState), errors.Is(err, db.ErrInsufficientStock):
			return ErrInvalidOrderState
		default:
			return err
		}
	}
	_ = s.store.CreateAuditLog(actor.ID, "order.update", "order", orderID,
		"status="+status+",partner="+partnerStatus+",delivery="+deliveryStatus)
	return nil
}

func (s *Service) TeamMembers() ([]TeamMember, error) {
	users, err := s.store.ListUsersByRoles([]string{db.RoleAdmin, db.RoleStaff, db.RoleUser})
	if err != nil {
		return nil, err
	}
	members := make([]TeamMember, 0, len(users))
	for _, u := range users {
		members = append(members, TeamMember{ID: u.ID, Username: u.Username, Email: u.Email, Role: u.Role, CreatedAt: u.CreatedAt})
	}
	return members, nil
}

func (s *Service) DeactivateUser(actor *Actor, userIDRaw string) error {
	if actor == nil || actor.Role != db.RoleAdmin {
		return ErrForbiddenUserAction
	}
	userID, err := parsePositiveInt(userIDRaw)
	if err != nil {
		return ErrInvalidUserID
	}
	if userID == actor.ID {
		return ErrProtectedUser
	}
	target, err := s.store.GetUserByID(userID)
	if err != nil {
		return err
	}
	if target == nil {
		return ErrUserNotFound
	}
	if target.Role == db.RoleAdmin {
		return ErrProtectedUser
	}
	if err := s.store.DeleteUser(userID); err != nil {
		return err
	}
	_ = s.store.CreateAuditLog(actor.ID, "user.deactivate", "user", userID, "deactivated via admin team")
	return nil
}

func (s *Service) AuditLogs(limit int) ([]AuditLog, error) {
	logs, err := s.store.ListAuditLogs(limit)
	if err != nil {
		return nil, err
	}
	mapped := make([]AuditLog, 0, len(logs))
	for _, entry := range logs {
		mapped = append(mapped, AuditLog{
			CreatedAt:   entry.CreatedAt,
			ActorUserID: entry.ActorUserID,
			ActorName:   entry.ActorName,
			Action:      entry.Action,
			TargetType:  entry.TargetType,
			TargetID:    entry.TargetID,
			Details:     entry.Details,
		})
	}
	return mapped, nil
}

func parsePositiveInt(raw string) (int, error) {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return 0, errors.New("invalid positive integer")
	}
	return value, nil
}

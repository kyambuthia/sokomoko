package admin

import (
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/kyambuthia/sokomoko/internal/db"
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

type store interface {
	CountActiveSessions() (int, error)
	CountProducts() (int, error)
	CountUsersByRole(role string) (int, error)
	CreateAuditLog(actorUserID int, action, targetType string, targetID int, details string) error
	DeleteUser(id int) error
	GetAllProducts() ([]db.Product, error)
	GetOrderStatusCounts() (map[string]int, error)
	GetUserByID(id int) (*db.User, error)
	ListAllOrders() ([]db.FulfillmentOrder, error)
	ListAuditLogs(limit int) ([]db.AuditLog, error)
	ListUsersByRoles(roles []string) ([]db.User, error)
	SumOrderRevenue() (float64, error)
	UpdateOrderByAdmin(orderID int, status, partnerStatus, deliveryStatus, deliveryNotice string) error
}

type Metrics struct {
	ProductCount int
	SessionCount int
	AdminCount   int
	StaffCount   int
	UserCount    int
}

type SalesReport struct {
	RevenueTotal   float64
	OrderCount     int
	PendingCount   int
	ShippedCount   int
	DeliveredCount int
}

type Product struct {
	Name          string
	Category      string
	Price         float64
	StockQuantity int
}

type OrderItem struct {
	ProductName string
	Quantity    int
	LineTotal   float64
}

type Order struct {
	ID              int
	CustomerName    string
	Status          string
	PartnerStatus   string
	DeliveryStatus  string
	DeliveryNotice  string
	DeliveryAddress string
	TotalAmount     float64
	Items           []OrderItem
}

type TeamMember struct {
	ID       int
	Username string
	Email    string
	Role     string
}

type AuditLog struct {
	CreatedAt   time.Time
	ActorUserID sql.NullInt64
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

func New(store store) *Service {
	return &Service{store: store}
}

func (s *Service) Metrics() (Metrics, error) {
	productCount, err := s.store.CountProducts()
	if err != nil {
		return Metrics{}, err
	}
	sessionCount, err := s.store.CountActiveSessions()
	if err != nil {
		return Metrics{}, err
	}
	adminCount, err := s.store.CountUsersByRole("admin")
	if err != nil {
		return Metrics{}, err
	}
	staffCount, err := s.store.CountUsersByRole("staff")
	if err != nil {
		return Metrics{}, err
	}
	userCount, err := s.store.CountUsersByRole("user")
	if err != nil {
		return Metrics{}, err
	}

	return Metrics{
		ProductCount: productCount,
		SessionCount: sessionCount,
		AdminCount:   adminCount,
		StaffCount:   staffCount,
		UserCount:    userCount,
	}, nil
}

func (s *Service) Products() ([]Product, error) {
	products, err := s.store.GetAllProducts()
	if err != nil {
		return nil, err
	}

	mapped := make([]Product, 0, len(products))
	for _, product := range products {
		mapped = append(mapped, Product{
			Name:          product.Name,
			Category:      product.Category,
			Price:         product.Price,
			StockQuantity: product.StockQuantity,
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
			items = append(items, OrderItem{
				ProductName: item.ProductName,
				Quantity:    item.Quantity,
				LineTotal:   item.LineTotal,
			})
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
			Items:           items,
		})
	}
	return mapped, nil
}

func (s *Service) Reports() (SalesReport, error) {
	revenueTotal, err := s.store.SumOrderRevenue()
	if err != nil {
		return SalesReport{}, err
	}
	statusCounts, err := s.store.GetOrderStatusCounts()
	if err != nil {
		return SalesReport{}, err
	}

	pendingCount := statusCounts["pending"] + statusCounts["processing"]
	shippedCount := statusCounts["shipped"]
	deliveredCount := statusCounts["delivered"]

	return SalesReport{
		RevenueTotal:   revenueTotal,
		PendingCount:   pendingCount,
		ShippedCount:   shippedCount,
		DeliveredCount: deliveredCount,
		OrderCount:     pendingCount + shippedCount + deliveredCount + statusCounts["cancelled"],
	}, nil
}

func (s *Service) UpdateOrder(actor *Actor, input UpdateOrderInput) error {
	if actor == nil || actor.Role != "admin" {
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
		case errors.Is(err, db.ErrInvalidOrderState):
			return ErrInvalidOrderState
		default:
			return err
		}
	}

	_ = s.store.CreateAuditLog(actor.ID, "order.update", "order", orderID, "status="+status+",partner="+partnerStatus+",delivery="+deliveryStatus)
	return nil
}

func (s *Service) TeamMembers() ([]TeamMember, error) {
	users, err := s.store.ListUsersByRoles([]string{"admin", "staff", "user"})
	if err != nil {
		return nil, err
	}

	mapped := make([]TeamMember, 0, len(users))
	for _, user := range users {
		mapped = append(mapped, TeamMember{
			ID:       user.ID,
			Username: user.Username,
			Email:    user.Email,
			Role:     user.Role,
		})
	}
	return mapped, nil
}

func (s *Service) DeactivateUser(actor *Actor, userIDRaw string) error {
	if actor == nil || actor.Role != "admin" {
		return ErrForbiddenUserAction
	}

	userID, err := parsePositiveInt(userIDRaw)
	if err != nil {
		return ErrInvalidUserID
	}

	targetUser, err := s.store.GetUserByID(userID)
	if err != nil {
		return err
	}
	if targetUser == nil {
		return ErrUserNotFound
	}
	if targetUser.Role == "admin" {
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

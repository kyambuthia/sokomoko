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

func New(store *db.Store) *Service {
	return &Service{store: newDBStore(store)}
}

func newWithStore(store store) *Service {
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
	return s.store.ListProducts()
}

func (s *Service) Orders() ([]Order, error) {
	return s.store.ListOrders()
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
	return s.store.ListTeamMembers()
}

func (s *Service) DeactivateUser(actor *Actor, userIDRaw string) error {
	if actor == nil || actor.Role != "admin" {
		return ErrForbiddenUserAction
	}

	userID, err := parsePositiveInt(userIDRaw)
	if err != nil {
		return ErrInvalidUserID
	}

	targetUser, err := s.store.GetTeamMember(userID)
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
	return s.store.ListAuditEntries(limit)
}

func parsePositiveInt(raw string) (int, error) {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return 0, errors.New("invalid positive integer")
	}
	return value, nil
}

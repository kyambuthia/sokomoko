package admin

import (
	"errors"
	"strconv"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/db"
)

var (
	ErrForbiddenOrderUpdate = errors.New("forbidden order update")
	ErrForbiddenUserAction  = errors.New("forbidden user action")
	ErrInvalidOrderID       = errors.New("invalid order id")
	ErrInvalidUserID        = errors.New("invalid user id")
	ErrUserNotFound         = errors.New("user not found")
	ErrProtectedUser        = errors.New("protected user")
)

type Service struct {
	store *db.Store
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

func (s *Service) Products() ([]db.Product, error) {
	return s.store.GetAllProducts()
}

func (s *Service) Orders() ([]db.FulfillmentOrder, error) {
	return s.store.ListAllOrders()
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

func (s *Service) UpdateOrder(actor *db.User, input UpdateOrderInput) error {
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
		return err
	}

	_ = s.store.CreateAuditLog(actor.ID, "order.update", "order", orderID, "status="+status+",partner="+partnerStatus+",delivery="+deliveryStatus)
	return nil
}

func (s *Service) TeamMembers() ([]db.User, error) {
	return s.store.ListUsersByRoles([]string{"admin", "staff", "user"})
}

func (s *Service) DeactivateUser(actor *db.User, userIDRaw string) error {
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

func (s *Service) AuditLogs(limit int) ([]db.AuditLog, error) {
	return s.store.ListAuditLogs(limit)
}

func parsePositiveInt(raw string) (int, error) {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return 0, errors.New("invalid positive integer")
	}
	return value, nil
}

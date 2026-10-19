// Package admin backs the platform administration workspace.
package admin

import (
	"context"
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
	Currency      string
	StockQuantity int
}

type OrderItem struct {
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
	PaymentMethod   string
	PaymentStatus   string
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

func (s *Service) Metrics(ctx context.Context) (Metrics, error) {
	c, err := s.store.DashboardCounts(ctx)
	if err != nil {
		return Metrics{}, err
	}
	return Metrics{
		ProductCount: c.Products, SessionCount: c.Sessions,
		AdminCount: c.Admins, StaffCount: c.Staff, UserCount: c.Users,
	}, nil
}

func (s *Service) Products(ctx context.Context) ([]Product, error) {
	products, err := s.store.GetAllProducts(ctx)
	if err != nil {
		return nil, err
	}
	mapped := make([]Product, 0, len(products))
	for _, p := range products {
		mapped = append(mapped, Product{
			ID: p.ID, Name: p.Name, Slug: p.Slug, Category: p.Category, PartnerName: p.PartnerName,
			Price: p.Price, Currency: p.Currency, StockQuantity: p.StockQuantity,
		})
	}
	return mapped, nil
}

func (s *Service) ProductsPage(ctx context.Context, page, limit int) ([]Product, int, error) {
	filter := db.ProductFilter{}
	total, err := s.store.CountFilteredProducts(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	filter.Limit, filter.Offset = limit, (page-1)*limit
	products, err := s.store.ListProducts(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	mapped := make([]Product, 0, len(products))
	for _, p := range products {
		mapped = append(mapped, Product{
			ID: p.ID, Name: p.Name, Slug: p.Slug, Category: p.Category, PartnerName: p.PartnerName,
			Price: p.Price, Currency: p.Currency, StockQuantity: p.StockQuantity,
		})
	}
	return mapped, total, nil
}

func (s *Service) Orders(ctx context.Context) ([]Order, error) {
	orders, err := s.store.ListAllOrders(ctx)
	if err != nil {
		return nil, err
	}
	mapped := make([]Order, 0, len(orders))
	for _, order := range orders {
		mapped = append(mapped, mapAdminOrder(order))
	}
	return mapped, nil
}

func (s *Service) OrdersPage(ctx context.Context, page, limit int) ([]Order, int, error) {
	orders, total, err := s.store.ListAllOrdersPage(ctx, limit, (page-1)*limit)
	if err != nil {
		return nil, 0, err
	}
	mapped := make([]Order, 0, len(orders))
	for _, order := range orders {
		mapped = append(mapped, mapAdminOrder(order))
	}
	return mapped, total, nil
}

func mapAdminOrder(order db.Order) Order {
	items := make([]OrderItem, 0, len(order.Items))
	for _, item := range order.Items {
		items = append(items, OrderItem{ProductName: item.ProductName, Quantity: item.Quantity, LineTotal: item.LineTotal})
	}
	return Order{
		ID: order.ID, Currency: order.Currency, CustomerName: order.CustomerName,
		Status: order.Status, PartnerStatus: order.PartnerStatus, DeliveryStatus: order.DeliveryStatus,
		DeliveryNotice: order.DeliveryNotice, DeliveryAddress: order.DeliveryAddress,
		TotalAmount: order.TotalAmount, CreatedAt: order.CreatedAt,
		PaymentMethod: order.PaymentMethod, PaymentStatus: order.PaymentStatus, Items: items,
	}
}

// DeliveryBoard summarizes the delivery pipeline: how many open orders are in
// each delivery status, and the orders still waiting to be delivered, oldest
// first.
type DeliveryBoard struct {
	QueuedCount     int
	ProcessingCount int
	ShippedCount    int
	DeliveredCount  int
	Pending         []Order
}

func (s *Service) Deliveries(ctx context.Context) (DeliveryBoard, error) {
	orders, err := s.Orders(ctx)
	if err != nil {
		return DeliveryBoard{}, err
	}
	var board DeliveryBoard
	for _, order := range orders {
		if order.Status == db.OrderStatusCancelled {
			continue
		}
		switch order.DeliveryStatus {
		case "queued":
			board.QueuedCount++
		case "processing":
			board.ProcessingCount++
		case "shipped":
			board.ShippedCount++
		case "delivered":
			board.DeliveredCount++
			continue
		}
		board.Pending = append(board.Pending, order)
	}
	// Orders arrive newest first; the board works oldest first.
	for i, j := 0, len(board.Pending)-1; i < j; i, j = i+1, j-1 {
		board.Pending[i], board.Pending[j] = board.Pending[j], board.Pending[i]
	}
	return board, nil
}

func (s *Service) Reports(ctx context.Context) (SalesReport, error) {
	revenue, err := s.store.SumOrderRevenue(ctx)
	if err != nil {
		return SalesReport{}, err
	}
	counts, err := s.store.GetOrderStatusCounts(ctx)
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

func (s *Service) UpdateOrder(ctx context.Context, actor *Actor, input UpdateOrderInput) error {
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

	if err := s.store.UpdateOrderByAdmin(ctx, orderID, status, partnerStatus, deliveryStatus, notice); err != nil {
		switch {
		case errors.Is(err, db.ErrOrderNotFound):
			return ErrOrderNotFound
		case errors.Is(err, db.ErrInvalidOrderState), errors.Is(err, db.ErrInsufficientStock):
			return ErrInvalidOrderState
		default:
			return err
		}
	}
	_ = s.store.CreateAuditLog(ctx, actor.ID, "order.update", "order", orderID,
		"status="+status+",partner="+partnerStatus+",delivery="+deliveryStatus)
	return nil
}

// TeamMembers returns one page of active accounts, optionally filtered by a
// username or email search, and the total that match.
func (s *Service) TeamMembers(ctx context.Context, query string, page, limit int) ([]TeamMember, int, error) {
	if page < 1 {
		page = 1
	}
	users, total, err := s.store.ListUsersByRoles(ctx, db.UserSearch{
		Roles:  []string{db.RoleAdmin, db.RoleStaff, db.RolePartner, db.RoleUser},
		Query:  query,
		Limit:  limit,
		Offset: (page - 1) * limit,
	})
	if err != nil {
		return nil, 0, err
	}
	members := make([]TeamMember, 0, len(users))
	for _, u := range users {
		members = append(members, TeamMember{ID: u.ID, Username: u.Username, Email: u.Email, Role: u.Role, CreatedAt: u.CreatedAt})
	}
	return members, total, nil
}

func (s *Service) DeactivateUser(ctx context.Context, actor *Actor, userIDRaw string) error {
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
	target, err := s.store.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if target == nil {
		return ErrUserNotFound
	}
	if target.Role == db.RoleAdmin {
		return ErrProtectedUser
	}
	if err := s.store.DeleteUser(ctx, userID); err != nil {
		return err
	}
	_ = s.store.CreateAuditLog(ctx, actor.ID, "user.deactivate", "user", userID, "deactivated via admin team")
	return nil
}

// DeactivatedMembers lists recently deactivated accounts an admin can restore.
func (s *Service) DeactivatedMembers(ctx context.Context) ([]TeamMember, error) {
	users, err := s.store.ListDeactivatedUsers(ctx, 50)
	if err != nil {
		return nil, err
	}
	members := make([]TeamMember, 0, len(users))
	for _, u := range users {
		members = append(members, TeamMember{ID: u.ID, Username: u.Username, Email: u.Email, Role: u.Role, CreatedAt: u.CreatedAt})
	}
	return members, nil
}

// ReactivateUser restores a deactivated account. The user signs in again with
// their existing password.
func (s *Service) ReactivateUser(ctx context.Context, actor *Actor, userIDRaw string) error {
	if actor == nil || actor.Role != db.RoleAdmin {
		return ErrForbiddenUserAction
	}
	userID, err := parsePositiveInt(userIDRaw)
	if err != nil {
		return ErrInvalidUserID
	}
	found, err := s.store.ReactivateUser(ctx, userID)
	if err != nil {
		return err
	}
	if !found {
		return ErrUserNotFound
	}
	_ = s.store.CreateAuditLog(ctx, actor.ID, "user.reactivate", "user", userID, "reactivated via admin team")
	return nil
}

func (s *Service) AuditLogs(ctx context.Context, limit int) ([]AuditLog, error) {
	logs, err := s.store.ListAuditLogs(ctx, limit)
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

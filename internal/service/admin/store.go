package admin

import "github.com/kyambuthia/sokomoko/internal/db"

type store interface {
	CountActiveSessions() (int, error)
	CountProducts() (int, error)
	CountUsersByRole(role string) (int, error)
	CreateAuditLog(actorUserID int, action, targetType string, targetID int, details string) error
	DeleteUser(id int) error
	GetOrderStatusCounts() (map[string]int, error)
	GetTeamMember(id int) (*TeamMember, error)
	ListAuditEntries(limit int) ([]AuditLog, error)
	ListOrders() ([]Order, error)
	ListProducts() ([]Product, error)
	ListTeamMembers() ([]TeamMember, error)
	SumOrderRevenue() (float64, error)
	UpdateOrderByAdmin(orderID int, status, partnerStatus, deliveryStatus, deliveryNotice string) error
}

type dbStore struct {
	store *db.Store
}

func newDBStore(store *db.Store) store {
	return &dbStore{store: store}
}

func (s *dbStore) CountActiveSessions() (int, error) {
	return s.store.CountActiveSessions()
}

func (s *dbStore) CountProducts() (int, error) {
	return s.store.CountProducts()
}

func (s *dbStore) CountUsersByRole(role string) (int, error) {
	return s.store.CountUsersByRole(role)
}

func (s *dbStore) CreateAuditLog(actorUserID int, action, targetType string, targetID int, details string) error {
	return s.store.CreateAuditLog(actorUserID, action, targetType, targetID, details)
}

func (s *dbStore) DeleteUser(id int) error {
	return s.store.DeleteUser(id)
}

func (s *dbStore) GetOrderStatusCounts() (map[string]int, error) {
	return s.store.GetOrderStatusCounts()
}

func (s *dbStore) GetTeamMember(id int) (*TeamMember, error) {
	user, err := s.store.GetUserByID(id)
	if err != nil || user == nil {
		return nil, err
	}
	member := mapTeamMember(*user)
	return &member, nil
}

func (s *dbStore) ListAuditEntries(limit int) ([]AuditLog, error) {
	logs, err := s.store.ListAuditLogs(limit)
	if err != nil {
		return nil, err
	}

	mapped := make([]AuditLog, 0, len(logs))
	for _, entry := range logs {
		mapped = append(mapped, mapAuditLog(entry))
	}
	return mapped, nil
}

func (s *dbStore) ListOrders() ([]Order, error) {
	orders, err := s.store.ListAllOrders()
	if err != nil {
		return nil, err
	}

	mapped := make([]Order, 0, len(orders))
	for _, order := range orders {
		mapped = append(mapped, mapOrder(order))
	}
	return mapped, nil
}

func (s *dbStore) ListProducts() ([]Product, error) {
	products, err := s.store.GetAllProducts()
	if err != nil {
		return nil, err
	}

	mapped := make([]Product, 0, len(products))
	for _, product := range products {
		mapped = append(mapped, mapProduct(product))
	}
	return mapped, nil
}

func (s *dbStore) ListTeamMembers() ([]TeamMember, error) {
	users, err := s.store.ListUsersByRoles([]string{"admin", "staff", "user"})
	if err != nil {
		return nil, err
	}

	mapped := make([]TeamMember, 0, len(users))
	for _, user := range users {
		mapped = append(mapped, mapTeamMember(user))
	}
	return mapped, nil
}

func (s *dbStore) SumOrderRevenue() (float64, error) {
	return s.store.SumOrderRevenue()
}

func (s *dbStore) UpdateOrderByAdmin(orderID int, status, partnerStatus, deliveryStatus, deliveryNotice string) error {
	return s.store.UpdateOrderByAdmin(orderID, status, partnerStatus, deliveryStatus, deliveryNotice)
}

func mapProduct(product db.Product) Product {
	return Product{
		Name:          product.Name,
		Category:      product.Category,
		Price:         product.Price,
		StockQuantity: product.StockQuantity,
	}
}

func mapOrder(order db.FulfillmentOrder) Order {
	items := make([]OrderItem, 0, len(order.Items))
	for _, item := range order.Items {
		items = append(items, OrderItem{
			ProductName: item.ProductName,
			Quantity:    item.Quantity,
			LineTotal:   item.LineTotal,
		})
	}

	return Order{
		ID:              order.ID,
		CustomerName:    order.CustomerName,
		Status:          order.Status,
		PartnerStatus:   order.PartnerStatus,
		DeliveryStatus:  order.DeliveryStatus,
		DeliveryNotice:  order.DeliveryNotice,
		DeliveryAddress: order.DeliveryAddress,
		TotalAmount:     order.TotalAmount,
		Items:           items,
	}
}

func mapTeamMember(user db.User) TeamMember {
	return TeamMember{
		ID:       user.ID,
		Username: user.Username,
		Email:    user.Email,
		Role:     user.Role,
	}
}

func mapAuditLog(entry db.AuditLog) AuditLog {
	return AuditLog{
		CreatedAt:   entry.CreatedAt,
		ActorUserID: entry.ActorUserID,
		Action:      entry.Action,
		TargetType:  entry.TargetType,
		TargetID:    entry.TargetID,
		Details:     entry.Details,
	}
}

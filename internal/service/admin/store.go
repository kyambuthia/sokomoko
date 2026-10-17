package admin

import (
	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/money"
)

type store interface {
	CountActiveSessions() (int, error)
	CountProductsByCategory() (map[int]int, error)
	CreateCategory(category db.Category) (int64, error)
	DeleteCategory(id int) error
	GetAllCategories() ([]db.Category, error)
	GetCategoryByID(id int) (*db.Category, error)
	CountProducts() (int, error)
	CountUsersByRole(role string) (int, error)
	CreateAuditLog(actorUserID int, action, targetType string, targetID int, details string) error
	DeleteUser(id int) error
	GetAllProducts() ([]db.Product, error)
	GetOrderStatusCounts() (map[string]int, error)
	GetUserByID(id int) (*db.User, error)
	ListAllOrders() ([]db.Order, error)
	ListAuditLogs(limit int) ([]db.AuditLog, error)
	ListUsersByRoles(roles []string) ([]db.User, error)
	SumOrderRevenue() (money.Cents, error)
	UpdateOrderByAdmin(orderID int, status, partnerStatus, deliveryStatus, deliveryNotice string) error
}

var _ store = (*db.Store)(nil)

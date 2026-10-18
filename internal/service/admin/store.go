package admin

import (
	"context"
	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/money"
)

type store interface {
	DashboardCounts() (db.DashboardCounts, error)
	CountProductsByCategory() (map[int]int, error)
	CreateCategory(category db.Category) (int64, error)
	CreatePartner(partner db.Partner) (int64, error)
	CreatePartnerUser(user db.User, partnerID int) (int64, error)
	DeleteCategory(id int) error
	GetAllCategories() ([]db.Category, error)
	GetCategoryByID(id int) (*db.Category, error)
	GetPartnerByID(id int) (*db.Partner, error)
	ListPartnerAccounts() ([]db.PartnerAccount, error)
	ListPartners() ([]db.Partner, error)
	SetPartnerStatus(id int, status string) (bool, error)
	CreateAuditLog(actorUserID int, action, targetType string, targetID int, details string) error
	DeleteUser(id int) error
	ListDeactivatedUsers(limit int) ([]db.User, error)
	ReactivateUser(id int) (bool, error)
	GetAllProducts() ([]db.Product, error)
	ListProductsContext(context.Context, db.ProductFilter) ([]db.Product, error)
	CountFilteredProductsContext(context.Context, db.ProductFilter) (int, error)
	GetOrderStatusCounts() (map[string]int, error)
	GetUserByID(id int) (*db.User, error)
	ListAllOrders() ([]db.Order, error)
	ListAllOrdersPageContext(context.Context, int, int) ([]db.Order, int, error)
	ListAuditLogs(limit int) ([]db.AuditLog, error)
	ListUsersByRoles(roles []string) ([]db.User, error)
	SumOrderRevenue() (money.Cents, error)
	UpdateOrderByAdmin(orderID int, status, partnerStatus, deliveryStatus, deliveryNotice string) error
}

var _ store = (*db.Store)(nil)

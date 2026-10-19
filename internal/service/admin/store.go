package admin

import (
	"context"
	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/money"
)

type store interface {
	DashboardCounts(ctx context.Context) (db.DashboardCounts, error)
	CountProductsByCategory(ctx context.Context) (map[int]int, error)
	CreateCategory(ctx context.Context, category db.Category) (int64, error)
	CreatePartner(ctx context.Context, partner db.Partner) (int64, error)
	CreatePartnerUser(ctx context.Context, user db.User, partnerID int) (int64, error)
	DeleteCategory(ctx context.Context, id int) error
	UpdateCategory(ctx context.Context, category db.Category) error
	UpdatePartnerDetails(ctx context.Context, id int, name, contactEmail string) (bool, error)
	GetAllCategories(ctx context.Context) ([]db.Category, error)
	GetCategoryByID(ctx context.Context, id int) (*db.Category, error)
	GetPartnerByID(ctx context.Context, id int) (*db.Partner, error)
	ListPartnerAccounts(ctx context.Context) ([]db.PartnerAccount, error)
	ListPartners(ctx context.Context) ([]db.Partner, error)
	SetPartnerStatus(ctx context.Context, id int, status string) (bool, error)
	CreateAuditLog(ctx context.Context, actorUserID int, action, targetType string, targetID int, details string) error
	DeleteUser(ctx context.Context, id int) error
	ListDeactivatedUsers(ctx context.Context, limit int) ([]db.User, error)
	ReactivateUser(ctx context.Context, id int) (bool, error)
	GetAllProducts(ctx context.Context) ([]db.Product, error)
	ListProducts(context.Context, db.ProductFilter) ([]db.Product, error)
	CountFilteredProducts(context.Context, db.ProductFilter) (int, error)
	GetOrderStatusCounts(ctx context.Context) (map[string]int, error)
	GetUserByID(ctx context.Context, id int) (*db.User, error)
	ListAllOrders(ctx context.Context) ([]db.Order, error)
	ListAllOrdersPage(context.Context, int, int) ([]db.Order, int, error)
	ListAuditLogs(ctx context.Context, limit int) ([]db.AuditLog, error)
	ListUsersByRoles(ctx context.Context, search db.UserSearch) ([]db.User, int, error)
	SumOrderRevenue(ctx context.Context) (money.Cents, error)
	UpdateOrderByAdmin(ctx context.Context, orderID int, status, partnerStatus, deliveryStatus, deliveryNotice string) error
}

var _ store = (*db.Store)(nil)

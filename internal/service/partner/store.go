package partner

import (
	"context"

	"github.com/kyambuthia/sokomoko/internal/db"
)

type store interface {
	PartnerIDForUser(ctx context.Context, userID int) (int, error)
	CountProducts(ctx context.Context) (int, error)
	CreateAuditLog(ctx context.Context, actorUserID int, action, targetType string, targetID int, details string) error
	CreateProduct(ctx context.Context, product db.Product) (int64, error)
	GetProductByID(ctx context.Context, id int) (*db.Product, error)
	UpdateProduct(ctx context.Context, product db.Product) error
	GetAllCategories(ctx context.Context) ([]db.Category, error)
	GetPartnerOrderSummary(ctx context.Context) (db.PartnerOrderSummary, error)
	GetStoreSettings(ctx context.Context) (*db.StoreSettings, error)
	ListOrdersForFulfillment(ctx context.Context) ([]db.Order, error)
	ListOrdersForPartner(ctx context.Context, partnerID int) ([]db.Order, error)
	PartnerOwnsOrder(ctx context.Context, partnerID, orderID int) (bool, error)
	ListPartners(ctx context.Context) ([]db.Partner, error)
	ListProducts(context.Context, db.ProductFilter) ([]db.Product, error)
	CountFilteredProducts(context.Context, db.ProductFilter) (int, error)
	ListFulfillmentOrdersPage(context.Context, int, string, int, int) ([]db.Order, int, error)
	FulfillmentSummary(context.Context, int) (db.PartnerOrderSummary, error)
	UpdateOrderFulfillment(ctx context.Context, orderID int, partnerStatus, deliveryStatus, deliveryNotice string) error
	UpsertStoreSettings(ctx context.Context, settings db.StoreSettings) error
}

var _ store = (*db.Store)(nil)

package partner

import (
	"context"

	"github.com/kyambuthia/sokomoko/internal/db"
)

type store interface {
	PartnerIDForUser(ctx context.Context, userID int) (int, error)
	CountProducts(ctx context.Context) (int, error)
	CreateAuditLog(ctx context.Context, actorUserID int, action, targetType string, targetID int, details string) error
	CreateProductForActor(ctx context.Context, product db.Product, actorID int, actorRole string) (int64, error)
	GetProductByID(ctx context.Context, id int) (*db.Product, error)
	UpdateProductForActor(ctx context.Context, product db.Product, actorID int, actorRole string) error
	DeleteProductForActor(context.Context, int, int, string) error
	GetAllCategories(ctx context.Context) ([]db.Category, error)
	GetPartnerOrderSummary(ctx context.Context) (db.PartnerOrderSummary, error)
	GetStoreSettings(ctx context.Context) (*db.StoreSettings, error)
	ListOrdersForFulfillment(ctx context.Context) ([]db.Order, error)
	ListOrdersForPartner(ctx context.Context, partnerID int) ([]db.Order, error)
	ListPartners(ctx context.Context) ([]db.Partner, error)
	ListProductsPage(context.Context, db.ProductFilter) ([]db.Product, int, error)
	CountFilteredProducts(context.Context, db.ProductFilter) (int, error)
	ListFulfillmentOrdersPage(context.Context, int, string, int, int) ([]db.Order, int, error)
	FulfillmentSummary(context.Context, int) (db.PartnerOrderSummary, error)
	UpdateOrderFulfillmentForActor(ctx context.Context, actorID int, actorRole string, orderID int, partnerStatus, deliveryStatus, deliveryNotice string) error
	UpsertStoreSettingsForActor(ctx context.Context, actorID int, actorRole string, settings db.StoreSettings) error
}

var _ store = (*db.Store)(nil)

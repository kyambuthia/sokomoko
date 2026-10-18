package partner

import (
	"context"

	"github.com/kyambuthia/sokomoko/internal/db"
)

type store interface {
	PartnerIDForUser(userID int) (int, error)
	CountProducts() (int, error)
	CreateAuditLog(actorUserID int, action, targetType string, targetID int, details string) error
	CreateProduct(product db.Product) (int64, error)
	GetProductByID(id int) (*db.Product, error)
	UpdateProduct(product db.Product) error
	GetAllCategories() ([]db.Category, error)
	GetPartnerOrderSummary() (db.PartnerOrderSummary, error)
	GetStoreSettings() (*db.StoreSettings, error)
	ListOrdersForFulfillment() ([]db.Order, error)
	ListOrdersForPartner(partnerID int) ([]db.Order, error)
	PartnerOwnsOrder(partnerID, orderID int) (bool, error)
	ListPartners() ([]db.Partner, error)
	ListProducts(filter db.ProductFilter) ([]db.Product, error)
	ListProductsContext(context.Context, db.ProductFilter) ([]db.Product, error)
	CountFilteredProductsContext(context.Context, db.ProductFilter) (int, error)
	ListFulfillmentOrdersPageContext(context.Context, int, string, int, int) ([]db.Order, int, error)
	FulfillmentSummaryContext(context.Context, int) (db.PartnerOrderSummary, error)
	UpdateOrderFulfillment(orderID int, partnerStatus, deliveryStatus, deliveryNotice string) error
	UpsertStoreSettings(settings db.StoreSettings) error
}

var _ store = (*db.Store)(nil)

package partner

import "github.com/kyambuthia/sokomoko/internal/db"

type store interface {
	CountProducts() (int, error)
	CreateAuditLog(actorUserID int, action, targetType string, targetID int, details string) error
	CreateProduct(product db.Product) (int64, error)
	GetAllCategories() ([]db.Category, error)
	GetPartnerOrderSummary() (db.PartnerOrderSummary, error)
	GetStoreSettings() (*db.StoreSettings, error)
	ListOrdersForFulfillment() ([]db.Order, error)
	ListPartners() ([]db.Partner, error)
	ListProducts(filter db.ProductFilter) ([]db.Product, error)
	UpdateOrderFulfillment(orderID int, partnerStatus, deliveryStatus, deliveryNotice string) error
	UpsertStoreSettings(settings db.StoreSettings) error
}

var _ store = (*db.Store)(nil)

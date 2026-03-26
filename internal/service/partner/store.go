package partner

import (
	"database/sql"

	"github.com/kyambuthia/sokomoko/internal/db"
)

type store interface {
	CountProducts() (int, error)
	CreateAuditLog(actorUserID int, action, targetType string, targetID int, details string) error
	CreateProduct(product productDraft) error
	GetCategories() ([]Category, error)
	GetOrderSummary() (Summary, error)
	GetProducts() ([]Product, error)
	GetStoreSettings() (*StoreSettings, error)
	ListOrdersForFulfillment() ([]Order, error)
	UpdateOrderFulfillment(orderID int, partnerStatus, deliveryStatus, deliveryNotice string) error
	SaveStoreSettings(settings StoreSettings) error
}

type dbStore struct {
	store *db.Store
}

type productDraft struct {
	Name          string
	Slug          string
	Description   string
	Price         float64
	StockQuantity int
	CategoryID    sql.NullInt64
}

func newDBStore(store *db.Store) store {
	return &dbStore{store: store}
}

func (s *dbStore) CountProducts() (int, error) {
	return s.store.CountProducts()
}

func (s *dbStore) CreateAuditLog(actorUserID int, action, targetType string, targetID int, details string) error {
	return s.store.CreateAuditLog(actorUserID, action, targetType, targetID, details)
}

func (s *dbStore) CreateProduct(product productDraft) error {
	_, err := s.store.CreateProduct(db.Product{
		Name:          product.Name,
		Slug:          product.Slug,
		Description:   product.Description,
		Price:         product.Price,
		StockQuantity: product.StockQuantity,
		CategoryID:    product.CategoryID,
	})
	return err
}

func (s *dbStore) GetCategories() ([]Category, error) {
	categories, err := s.store.GetAllCategories()
	if err != nil {
		return nil, err
	}

	mapped := make([]Category, 0, len(categories))
	for _, category := range categories {
		mapped = append(mapped, mapCategory(category))
	}
	return mapped, nil
}

func (s *dbStore) GetOrderSummary() (Summary, error) {
	summary, err := s.store.GetPartnerOrderSummary()
	if err != nil {
		return Summary{}, err
	}
	return mapSummary(summary), nil
}

func (s *dbStore) GetProducts() ([]Product, error) {
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

func (s *dbStore) GetStoreSettings() (*StoreSettings, error) {
	settings, err := s.store.GetStoreSettings()
	if err != nil || settings == nil {
		return nil, err
	}

	mapped := mapStoreSettings(*settings)
	return &mapped, nil
}

func (s *dbStore) ListOrdersForFulfillment() ([]Order, error) {
	orders, err := s.store.ListOrdersForFulfillment()
	if err != nil {
		return nil, err
	}

	mapped := make([]Order, 0, len(orders))
	for _, order := range orders {
		mapped = append(mapped, mapOrder(order))
	}
	return mapped, nil
}

func (s *dbStore) SaveStoreSettings(settings StoreSettings) error {
	return s.store.UpsertStoreSettings(db.StoreSettings{
		StoreName:    settings.StoreName,
		StoreSlug:    settings.StoreSlug,
		Description:  settings.Description,
		ContactEmail: settings.ContactEmail,
	})
}

func (s *dbStore) UpdateOrderFulfillment(orderID int, partnerStatus, deliveryStatus, deliveryNotice string) error {
	return s.store.UpdateOrderFulfillment(orderID, partnerStatus, deliveryStatus, deliveryNotice)
}

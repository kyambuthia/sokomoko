package partner

import (
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/kyambuthia/sokomoko/internal/db"
)

var (
	ErrStoreNotConfigured        = errors.New("store not configured")
	ErrMissingStoreFields        = errors.New("missing store fields")
	ErrForbiddenOrderUpdate      = errors.New("forbidden order update")
	ErrInvalidOrderID            = errors.New("invalid order id")
	ErrOrderNotFound             = errors.New("order not found")
	ErrInvalidOrderTransition    = errors.New("invalid order transition")
	ErrMissingProductFields      = errors.New("missing product fields")
	ErrInvalidProductPrice       = errors.New("invalid product price")
	ErrInvalidProductStock       = errors.New("invalid product stock")
	ErrUnableToCreateProduct     = errors.New("unable to create product")
	ErrUnableToCreateProductSlug = errors.New("unable to create product slug")
)

type Service struct {
	store store
}

type store interface {
	CountProducts() (int, error)
	CreateAuditLog(actorUserID int, action, targetType string, targetID int, details string) error
	CreateProduct(product db.Product) (int64, error)
	GetAllCategories() ([]db.Category, error)
	GetAllProducts() ([]db.Product, error)
	GetPartnerOrderSummary() (db.PartnerOrderSummary, error)
	GetStoreSettings() (*db.StoreSettings, error)
	ListOrdersForFulfillment() ([]db.FulfillmentOrder, error)
	UpdateOrderFulfillment(orderID int, partnerStatus, deliveryStatus, deliveryNotice string) error
	UpsertStoreSettings(settings db.StoreSettings) error
}

type StoreSettingsInput struct {
	StoreName    string
	StoreSlug    string
	Description  string
	ContactEmail string
}

type DashboardData struct {
	Settings     db.StoreSettings
	ProductCount int
	OrderSummary db.PartnerOrderSummary
}

type ProductsData struct {
	Settings   db.StoreSettings
	Products   []db.Product
	Categories []db.Category
}

type OrdersData struct {
	Filter  string
	Orders  []db.FulfillmentOrder
	Summary db.PartnerOrderSummary
}

type CreateProductInput struct {
	Name        string
	Description string
	Price       string
	Stock       string
	CategoryID  string
}

type UpdateOrderInput struct {
	OrderID        string
	PartnerStatus  string
	DeliveryStatus string
	DeliveryNotice string
}

func New(store store) *Service {
	return &Service{store: store}
}

func (s *Service) StoreSettings() (*db.StoreSettings, error) {
	return s.store.GetStoreSettings()
}

func (s *Service) SaveStoreSettings(input StoreSettingsInput) (db.StoreSettings, error) {
	settings := db.StoreSettings{
		StoreName:    strings.TrimSpace(input.StoreName),
		StoreSlug:    strings.ToLower(strings.TrimSpace(input.StoreSlug)),
		Description:  strings.TrimSpace(input.Description),
		ContactEmail: strings.ToLower(strings.TrimSpace(input.ContactEmail)),
	}

	if settings.StoreName == "" || settings.StoreSlug == "" || settings.ContactEmail == "" {
		return settings, ErrMissingStoreFields
	}

	if err := s.store.UpsertStoreSettings(settings); err != nil {
		return settings, err
	}

	return settings, nil
}

func (s *Service) Dashboard() (DashboardData, error) {
	settings, err := s.requireStoreSettings()
	if err != nil {
		return DashboardData{}, err
	}

	productCount, err := s.store.CountProducts()
	if err != nil {
		return DashboardData{}, err
	}
	orderSummary, err := s.store.GetPartnerOrderSummary()
	if err != nil {
		return DashboardData{}, err
	}

	return DashboardData{
		Settings:     *settings,
		ProductCount: productCount,
		OrderSummary: orderSummary,
	}, nil
}

func (s *Service) Products() (ProductsData, error) {
	settings, err := s.requireStoreSettings()
	if err != nil {
		return ProductsData{}, err
	}

	products, err := s.store.GetAllProducts()
	if err != nil {
		return ProductsData{}, err
	}
	categories, err := s.store.GetAllCategories()
	if err != nil {
		categories = nil
	}

	return ProductsData{
		Settings:   *settings,
		Products:   products,
		Categories: categories,
	}, nil
}

func (s *Service) CreateProduct(input CreateProductInput) error {
	if _, err := s.requireStoreSettings(); err != nil {
		return err
	}

	name := strings.TrimSpace(input.Name)
	description := strings.TrimSpace(input.Description)
	priceRaw := strings.TrimSpace(input.Price)
	stockRaw := strings.TrimSpace(input.Stock)
	categoryIDRaw := strings.TrimSpace(input.CategoryID)

	if name == "" || priceRaw == "" || stockRaw == "" {
		return ErrMissingProductFields
	}

	price, err := strconv.ParseFloat(priceRaw, 64)
	if err != nil || price < 0 {
		return ErrInvalidProductPrice
	}
	stock, err := strconv.Atoi(stockRaw)
	if err != nil || stock < 0 {
		return ErrInvalidProductStock
	}

	slugBase := slugify(name)
	if slugBase == "" {
		slugBase = "product"
	}

	product := db.Product{
		Name:          name,
		Slug:          slugBase,
		Description:   description,
		Price:         price,
		StockQuantity: stock,
	}

	if categoryIDRaw != "" {
		categoryID, parseErr := strconv.Atoi(categoryIDRaw)
		if parseErr == nil && categoryID > 0 {
			product.CategoryID = sql.NullInt64{Int64: int64(categoryID), Valid: true}
		}
	}

	for attempt := 0; attempt < 10; attempt++ {
		if attempt > 0 {
			product.Slug = slugBase + "-" + strconv.FormatInt(time.Now().Unix(), 10) + "-" + strconv.Itoa(attempt)
		}
		if _, err := s.store.CreateProduct(product); err != nil {
			if errors.Is(err, db.ErrProductSlugConflict) {
				continue
			}
			return ErrUnableToCreateProduct
		}
		return nil
	}

	return ErrUnableToCreateProductSlug
}

func (s *Service) Orders(filter string) (OrdersData, error) {
	cleanFilter := strings.TrimSpace(filter)
	if cleanFilter == "" {
		cleanFilter = "all"
	}

	orders, err := s.store.ListOrdersForFulfillment()
	if err != nil {
		return OrdersData{}, err
	}

	filtered := make([]db.FulfillmentOrder, 0, len(orders))
	for _, order := range orders {
		if cleanFilter == "all" || order.PartnerStatus == cleanFilter {
			filtered = append(filtered, order)
		}
	}

	summary, err := s.store.GetPartnerOrderSummary()
	if err != nil {
		return OrdersData{}, err
	}

	return OrdersData{
		Filter:  cleanFilter,
		Orders:  filtered,
		Summary: summary,
	}, nil
}

func (s *Service) UpdateOrder(actor *db.User, input UpdateOrderInput) error {
	if actor == nil || (actor.Role != "admin" && actor.Role != "staff") {
		return ErrForbiddenOrderUpdate
	}

	orderID, err := strconv.Atoi(strings.TrimSpace(input.OrderID))
	if err != nil || orderID <= 0 {
		return ErrInvalidOrderID
	}

	partnerStatus := strings.TrimSpace(input.PartnerStatus)
	deliveryStatus := strings.TrimSpace(input.DeliveryStatus)
	note := strings.TrimSpace(input.DeliveryNotice)

	if err := s.store.UpdateOrderFulfillment(orderID, partnerStatus, deliveryStatus, note); err != nil {
		switch {
		case errors.Is(err, db.ErrOrderNotFound):
			return ErrOrderNotFound
		case errors.Is(err, db.ErrInvalidPartnerTransition),
			errors.Is(err, db.ErrInvalidDeliveryTransition),
			errors.Is(err, db.ErrInvalidPartnerStatus),
			errors.Is(err, db.ErrInvalidDeliveryStatus):
			return ErrInvalidOrderTransition
		default:
			return err
		}
	}

	_ = s.store.CreateAuditLog(actor.ID, "partner.fulfillment.update", "order", orderID, "partner="+partnerStatus+",delivery="+deliveryStatus)
	return nil
}

func (s *Service) requireStoreSettings() (*db.StoreSettings, error) {
	settings, err := s.store.GetStoreSettings()
	if err != nil {
		return nil, err
	}
	if settings == nil {
		return nil, ErrStoreNotConfigured
	}
	return settings, nil
}

func slugify(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}

	var b strings.Builder
	prevDash := false
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prevDash = false
			continue
		}
		if !prevDash {
			b.WriteByte('-')
			prevDash = true
		}
	}

	return strings.Trim(b.String(), "-")
}

func isProductSlugConflict(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, db.ErrProductSlugConflict)
}

// Package partner backs the partner workspace: store setup, catalog entry, and
// order fulfillment.
package partner

import (
	"database/sql"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/money"
)

var (
	ErrStoreNotConfigured        = errors.New("store not configured")
	ErrMissingStoreFields        = errors.New("missing store fields")
	ErrInvalidStoreSlug          = errors.New("invalid store slug")
	ErrInvalidContactEmail       = errors.New("invalid contact email")
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

const (
	maxProductPrice  money.Cents = 10_000_000 // 100,000.00
	maxProductStock              = 1_000_000
	maxProductName               = 200
	maxSlugLength                = 80
	maxNoticeLength              = 500
	maxDescriptionLn             = 5000
)

var storeSlugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

type Service struct {
	store store
}

type StoreSettingsInput struct {
	StoreName    string
	StoreSlug    string
	Description  string
	ContactEmail string
}

type StoreSettings struct {
	StoreName    string
	StoreSlug    string
	Description  string
	ContactEmail string
}

type Summary struct {
	NewCount        int
	InProgressCount int
	DispatchedCount int
	CompletedCount  int
	OverdueCount    int
}

type Product struct {
	ID            int
	Name          string
	Slug          string
	Category      string
	PartnerName   string
	Price         money.Cents
	StockQuantity int
}

type Category struct {
	ID   int
	Name string
}

type PartnerOption struct {
	ID   int
	Name string
}

type OrderItem struct {
	ProductName string
	Quantity    int
	LineTotal   money.Cents
}

type Order struct {
	ID              int
	CustomerName    string
	Status          string
	PartnerStatus   string
	DeliveryStatus  string
	DeliveryNotice  string
	DeliveryAddress string
	TotalAmount     money.Cents
	CreatedAt       time.Time
	Items           []OrderItem
}

type Actor struct {
	ID   int
	Role string
}

type DashboardData struct {
	Settings     StoreSettings
	ProductCount int
	OrderSummary Summary
}

type ProductsData struct {
	Settings   StoreSettings
	Products   []Product
	Categories []Category
	Partners   []PartnerOption
}

type OrdersData struct {
	Filter  string
	Orders  []Order
	Summary Summary
}

type CreateProductInput struct {
	Name        string
	Description string
	Price       string
	Stock       string
	CategoryID  string
	PartnerID   string
}

type UpdateOrderInput struct {
	OrderID        string
	PartnerStatus  string
	DeliveryStatus string
	DeliveryNotice string
}

func New(store *db.Store) *Service {
	return &Service{store: store}
}

func newWithStore(store store) *Service {
	return &Service{store: store}
}

func (s *Service) StoreSettings() (*StoreSettings, error) {
	settings, err := s.store.GetStoreSettings()
	if err != nil || settings == nil {
		return nil, err
	}
	mapped := mapStoreSettings(*settings)
	return &mapped, nil
}

func (s *Service) SaveStoreSettings(input StoreSettingsInput) (StoreSettings, error) {
	settings := StoreSettings{
		StoreName:    strings.TrimSpace(input.StoreName),
		StoreSlug:    strings.ToLower(strings.TrimSpace(input.StoreSlug)),
		Description:  strings.TrimSpace(input.Description),
		ContactEmail: strings.ToLower(strings.TrimSpace(input.ContactEmail)),
	}
	if settings.StoreName == "" || settings.StoreSlug == "" || settings.ContactEmail == "" {
		return settings, ErrMissingStoreFields
	}
	if !storeSlugPattern.MatchString(settings.StoreSlug) || len(settings.StoreSlug) > maxSlugLength {
		return settings, ErrInvalidStoreSlug
	}
	if addr, err := mail.ParseAddress(settings.ContactEmail); err != nil || addr.Address != settings.ContactEmail {
		return settings, ErrInvalidContactEmail
	}
	err := s.store.UpsertStoreSettings(db.StoreSettings{
		StoreName:    settings.StoreName,
		StoreSlug:    settings.StoreSlug,
		Description:  settings.Description,
		ContactEmail: settings.ContactEmail,
	})
	return settings, err
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
	summary, err := s.store.GetPartnerOrderSummary()
	if err != nil {
		return DashboardData{}, err
	}
	return DashboardData{
		Settings:     *settings,
		ProductCount: productCount,
		OrderSummary: Summary(summary),
	}, nil
}

func (s *Service) Products() (ProductsData, error) {
	settings, err := s.requireStoreSettings()
	if err != nil {
		return ProductsData{}, err
	}
	products, err := s.store.ListProducts(db.ProductFilter{})
	if err != nil {
		return ProductsData{}, err
	}
	categories, err := s.store.GetAllCategories()
	if err != nil {
		return ProductsData{}, err
	}
	partners, err := s.store.ListPartners()
	if err != nil {
		return ProductsData{}, err
	}

	data := ProductsData{Settings: *settings}
	for _, p := range products {
		data.Products = append(data.Products, Product{
			ID: p.ID, Name: p.Name, Slug: p.Slug, Category: p.Category, PartnerName: p.PartnerName,
			Price: p.Price, StockQuantity: p.StockQuantity,
		})
	}
	for _, c := range categories {
		data.Categories = append(data.Categories, Category{ID: c.ID, Name: c.Name})
	}
	for _, p := range partners {
		if p.Status == "active" {
			data.Partners = append(data.Partners, PartnerOption{ID: p.ID, Name: p.Name})
		}
	}
	return data, nil
}

func (s *Service) CreateProduct(input CreateProductInput) error {
	if _, err := s.requireStoreSettings(); err != nil {
		return err
	}

	name := strings.TrimSpace(input.Name)
	description := strings.TrimSpace(input.Description)
	priceRaw := strings.TrimSpace(input.Price)
	stockRaw := strings.TrimSpace(input.Stock)
	if name == "" || priceRaw == "" || stockRaw == "" {
		return ErrMissingProductFields
	}
	if len([]rune(name)) > maxProductName || len([]rune(description)) > maxDescriptionLn {
		return ErrMissingProductFields
	}

	price, err := money.Parse(priceRaw)
	if err != nil || price > maxProductPrice {
		return ErrInvalidProductPrice
	}
	stock, err := strconv.Atoi(stockRaw)
	if err != nil || stock < 0 || stock > maxProductStock {
		return ErrInvalidProductStock
	}

	product := db.Product{
		Name:          name,
		Description:   description,
		Price:         price,
		StockQuantity: stock,
		CategoryID:    parseOptionalID(input.CategoryID),
		PartnerID:     parseOptionalID(input.PartnerID),
	}

	slugBase := slugify(name)
	if slugBase == "" {
		slugBase = "product"
	}
	for attempt := 0; attempt < 10; attempt++ {
		product.Slug = slugBase
		if attempt > 0 {
			product.Slug = fmt.Sprintf("%s-%d-%d", slugBase, time.Now().Unix(), attempt)
		}
		_, err := s.store.CreateProduct(product)
		if err == nil {
			return nil
		}
		if !errors.Is(err, db.ErrProductSlugConflict) {
			return fmt.Errorf("%w: %w", ErrUnableToCreateProduct, err)
		}
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
	filtered := make([]Order, 0, len(orders))
	for _, order := range orders {
		if cleanFilter == "all" || order.PartnerStatus == cleanFilter {
			filtered = append(filtered, mapOrder(order))
		}
	}
	summary, err := s.store.GetPartnerOrderSummary()
	if err != nil {
		return OrdersData{}, err
	}
	return OrdersData{Filter: cleanFilter, Orders: filtered, Summary: Summary(summary)}, nil
}

func (s *Service) UpdateOrder(actor *Actor, input UpdateOrderInput) error {
	if actor == nil || (actor.Role != db.RoleAdmin && actor.Role != db.RoleStaff) {
		return ErrForbiddenOrderUpdate
	}
	orderID, err := strconv.Atoi(strings.TrimSpace(input.OrderID))
	if err != nil || orderID <= 0 {
		return ErrInvalidOrderID
	}
	partnerStatus := strings.TrimSpace(input.PartnerStatus)
	deliveryStatus := strings.TrimSpace(input.DeliveryStatus)
	notice := strings.TrimSpace(input.DeliveryNotice)
	if runes := []rune(notice); len(runes) > maxNoticeLength {
		notice = string(runes[:maxNoticeLength])
	}

	if err := s.store.UpdateOrderFulfillment(orderID, partnerStatus, deliveryStatus, notice); err != nil {
		switch {
		case errors.Is(err, db.ErrOrderNotFound):
			return ErrOrderNotFound
		case errors.Is(err, db.ErrInvalidPartnerTransition),
			errors.Is(err, db.ErrInvalidDeliveryTransition),
			errors.Is(err, db.ErrInvalidPartnerStatus),
			errors.Is(err, db.ErrInvalidDeliveryStatus),
			errors.Is(err, db.ErrInsufficientStock):
			return ErrInvalidOrderTransition
		default:
			return err
		}
	}
	_ = s.store.CreateAuditLog(actor.ID, "partner.fulfillment.update", "order", orderID,
		"partner="+partnerStatus+",delivery="+deliveryStatus)
	return nil
}

func (s *Service) requireStoreSettings() (*StoreSettings, error) {
	settings, err := s.StoreSettings()
	if err != nil {
		return nil, err
	}
	if settings == nil {
		return nil, ErrStoreNotConfigured
	}
	return settings, nil
}

func parseOptionalID(raw string) sql.NullInt64 {
	id, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || id <= 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(id), Valid: true}
}

func mapStoreSettings(settings db.StoreSettings) StoreSettings {
	return StoreSettings{
		StoreName:    settings.StoreName,
		StoreSlug:    settings.StoreSlug,
		Description:  settings.Description,
		ContactEmail: settings.ContactEmail,
	}
}

func mapOrder(order db.Order) Order {
	items := make([]OrderItem, 0, len(order.Items))
	for _, item := range order.Items {
		items = append(items, OrderItem{ProductName: item.ProductName, Quantity: item.Quantity, LineTotal: item.LineTotal})
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
		CreatedAt:       order.CreatedAt,
		Items:           items,
	}
}

// slugify produces an ASCII slug matching the products.slug constraint:
// lower-case letters and digits separated by single dashes.
func slugify(value string) string {
	var b strings.Builder
	pendingDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		isAlnum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if !isAlnum {
			pendingDash = b.Len() > 0
			continue
		}
		if pendingDash {
			b.WriteByte('-')
			pendingDash = false
		}
		b.WriteRune(r)
		if b.Len() >= maxSlugLength {
			break
		}
	}
	return strings.TrimRight(b.String(), "-")
}

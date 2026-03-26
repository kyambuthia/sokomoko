package partner

import (
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
	Category      string
	Price         float64
	StockQuantity int
}

type Category struct {
	ID   int
	Name string
}

type OrderItem struct {
	ProductName string
	Quantity    int
	LineTotal   float64
}

type Order struct {
	ID              int
	CustomerName    string
	Status          string
	PartnerStatus   string
	DeliveryStatus  string
	DeliveryNotice  string
	DeliveryAddress string
	TotalAmount     float64
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
}

type UpdateOrderInput struct {
	OrderID        string
	PartnerStatus  string
	DeliveryStatus string
	DeliveryNotice string
}

func New(store *db.Store) *Service {
	return &Service{store: newDBStore(store)}
}

func newWithStore(store store) *Service {
	return &Service{store: store}
}

func (s *Service) StoreSettings() (*StoreSettings, error) {
	return s.store.GetStoreSettings()
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

	if err := s.store.SaveStoreSettings(settings); err != nil {
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
	orderSummary, err := s.store.GetOrderSummary()
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

	products, err := s.store.GetProducts()
	if err != nil {
		return ProductsData{}, err
	}
	categories, err := s.store.GetCategories()
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

	product := productDraft{
		Name:          name,
		Slug:          slugBase,
		Description:   description,
		Price:         price,
		StockQuantity: stock,
	}

	if categoryIDRaw != "" {
		categoryID, parseErr := strconv.Atoi(categoryIDRaw)
		if parseErr == nil && categoryID > 0 {
			product.CategoryID.Int64 = int64(categoryID)
			product.CategoryID.Valid = true
		}
	}

	for attempt := 0; attempt < 10; attempt++ {
		if attempt > 0 {
			product.Slug = slugBase + "-" + strconv.FormatInt(time.Now().Unix(), 10) + "-" + strconv.Itoa(attempt)
		}
		if err := s.store.CreateProduct(product); err != nil {
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

	filtered := make([]Order, 0, len(orders))
	for _, order := range orders {
		if cleanFilter == "all" || order.PartnerStatus == cleanFilter {
			filtered = append(filtered, order)
		}
	}

	summary, err := s.store.GetOrderSummary()
	if err != nil {
		return OrdersData{}, err
	}

	return OrdersData{
		Filter:  cleanFilter,
		Orders:  filtered,
		Summary: summary,
	}, nil
}

func (s *Service) UpdateOrder(actor *Actor, input UpdateOrderInput) error {
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

func (s *Service) requireStoreSettings() (*StoreSettings, error) {
	settings, err := s.store.GetStoreSettings()
	if err != nil {
		return nil, err
	}
	if settings == nil {
		return nil, ErrStoreNotConfigured
	}
	return settings, nil
}

func mapStoreSettings(settings db.StoreSettings) StoreSettings {
	return StoreSettings{
		StoreName:    settings.StoreName,
		StoreSlug:    settings.StoreSlug,
		Description:  settings.Description,
		ContactEmail: settings.ContactEmail,
	}
}

func mapProduct(product db.Product) Product {
	return Product{
		ID:            product.ID,
		Name:          product.Name,
		Category:      product.Category,
		Price:         product.Price,
		StockQuantity: product.StockQuantity,
	}
}

func mapCategory(category db.Category) Category {
	return Category{
		ID:   int(category.ID),
		Name: category.Name,
	}
}

func mapSummary(summary db.PartnerOrderSummary) Summary {
	return Summary{
		NewCount:        summary.NewCount,
		InProgressCount: summary.InProgressCount,
		DispatchedCount: summary.DispatchedCount,
		CompletedCount:  summary.CompletedCount,
		OverdueCount:    summary.OverdueCount,
	}
}

func mapOrder(order db.FulfillmentOrder) Order {
	items := make([]OrderItem, 0, len(order.Items))
	for _, item := range order.Items {
		items = append(items, OrderItem{
			ProductName: item.ProductName,
			Quantity:    item.Quantity,
			LineTotal:   item.LineTotal,
		})
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
		Items:           items,
	}
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

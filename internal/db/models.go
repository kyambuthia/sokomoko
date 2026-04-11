package db

import (
	"database/sql"
	"strings"
	"time"
)

const DefaultProductImageURL = "/static/images/gray_fabric_bluetooth_speaker.png"

var bundledProductImageURLs = []string{
	"/static/images/black_adjustable_desk_lamp.png",
	"/static/images/black_over_ear_headphones.png",
	"/static/images/gray_fabric_bluetooth_speaker.png",
	"/static/images/iphone_category_card.png",
	"/static/images/product_tv.png",
	"/static/images/stainless_steel_electric_kettle.png",
	"/static/images/white_wireless_earbuds_charging_case.png",
}

// ResolveProductImageURL normalizes product image URLs to bundled local assets.
func ResolveProductImageURL(rawURL, slug, name, category string) string {
	cleanURL := strings.TrimSpace(rawURL)
	switch {
	case cleanURL == "":
		return fallbackProductImageURL(slug, name, category)
	case strings.HasPrefix(cleanURL, "data:"):
		return cleanURL
	case strings.HasPrefix(cleanURL, "/"):
		return cleanURL
	case strings.HasPrefix(cleanURL, "static/"):
		return "/" + cleanURL
	case strings.HasPrefix(cleanURL, "http://"), strings.HasPrefix(cleanURL, "https://"):
		return fallbackProductImageURL(slug, name, category)
	default:
		return "/" + strings.TrimLeft(cleanURL, "/")
	}
}

func fallbackProductImageURL(slug, name, category string) string {
	descriptor := strings.ToLower(strings.TrimSpace(strings.Join([]string{slug, name, category}, " ")))
	switch {
	case strings.Contains(descriptor, "earbud"):
		return "/static/images/white_wireless_earbuds_charging_case.png"
	case strings.Contains(descriptor, "headphone"):
		return "/static/images/black_over_ear_headphones.png"
	case strings.Contains(descriptor, "speaker"):
		return "/static/images/gray_fabric_bluetooth_speaker.png"
	case strings.Contains(descriptor, "lamp"):
		return "/static/images/black_adjustable_desk_lamp.png"
	case strings.Contains(descriptor, "kettle"):
		return "/static/images/stainless_steel_electric_kettle.png"
	case strings.Contains(descriptor, "tv"), strings.Contains(descriptor, "television"), strings.Contains(descriptor, "monitor"):
		return "/static/images/product_tv.png"
	}

	if descriptor == "" {
		return DefaultProductImageURL
	}

	sum := 0
	for _, r := range descriptor {
		sum += int(r)
	}
	return bundledProductImageURLs[sum%len(bundledProductImageURLs)]
}

// User represents a user in the system
type User struct {
	ID           int
	Username     string
	Email        string
	PasswordHash string
	Salt         string
	Role         string
	Slug         string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    sql.NullTime
}

// Category represents a product category
type Category struct {
	ID          int
	Name        string
	Slug        string
	Description string
	ParentID    sql.NullInt64
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   sql.NullTime
}

// Product represents a product in the system
type Product struct {
	ID            int
	Name          string
	Slug          string
	Description   string
	Price         float64
	StockQuantity int
	Category      string
	CategoryID    sql.NullInt64
	PartnerID     sql.NullInt64
	PartnerName   string
	Images        []ProductImage
	CreatedAt     time.Time
	UpdatedAt     time.Time
	DeletedAt     sql.NullTime
}

// GetPrimaryImageURL returns the first image URL or a placeholder
func (p Product) GetPrimaryImageURL() string {
	imageURL := ""
	if len(p.Images) > 0 {
		imageURL = p.Images[0].URL
	}
	return ResolveProductImageURL(imageURL, p.Slug, p.Name, p.Category)
}

// ProductImage represents an image for a product
type ProductImage struct {
	ID           int
	ProductID    int
	URL          string
	AltText      string
	DisplayOrder int
	CreatedAt    time.Time
}

type Warehouse struct {
	ID        int
	Name      string
	Slug      string
	IsDefault bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

type InventoryStock struct {
	ID                int
	ProductID         int
	WarehouseID       int
	OnHandQuantity    int
	ReservedQuantity  int
	AllocatedQuantity int
	AvailableQuantity int
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type StockReservation struct {
	ID             int
	ProductID      int
	WarehouseID    int
	UserID         sql.NullInt64
	ReservationKey sql.NullString
	Quantity       int
	Status         string
	ExpiresAt      sql.NullTime
	ReleasedAt     sql.NullTime
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type StockMovement struct {
	ID            int
	ProductID     int
	WarehouseID   int
	MovementType  string
	QuantityDelta int
	Note          sql.NullString
	CreatedAt     time.Time
}

// Session represents a persistent user session
type Session struct {
	ID        string
	UserID    int
	ExpiresAt time.Time
	CreatedAt time.Time
}

type PasswordResetToken struct {
	ID        int
	TokenHash string
	UserID    int
	ExpiresAt time.Time
	UsedAt    sql.NullTime
	CreatedAt time.Time
}

type StoreSettings struct {
	ID            int
	StoreName     string
	StoreSlug     string
	Description   string
	ContactEmail  string
	InitializedAt time.Time
	UpdatedAt     time.Time
}

type CartItem struct {
	ProductID       int
	ProductName     string
	ProductSlug     string
	ProductImageURL string
	UnitPrice       float64
	Quantity        int
	StockQuantity   int
	LineTotal       float64
}

type OrderItem struct {
	ProductID   int
	ProductName string
	Quantity    int
	UnitPrice   float64
	LineTotal   float64
}

type CustomerOrder struct {
	ID              int
	Status          string
	PartnerStatus   string
	DeliveryStatus  string
	DeliveryAddress string
	DeliveryNotice  string
	TotalAmount     float64
	CreatedAt       time.Time
	Items           []OrderItem
}

const (
	CheckoutStatusOpen      = "open"
	CheckoutStatusCompleted = "completed"
	CheckoutStatusExpired   = "expired"
	CheckoutStatusCancelled = "cancelled"
)

type Checkout struct {
	ID              int
	UserID          int
	Token           string
	Status          string
	Currency        string
	PaymentMethod   string
	SubtotalAmount  float64
	ShippingFee     float64
	TaxAmount       float64
	TotalAmount     float64
	DeliveryAddress sql.NullString
	ExpiresAt       sql.NullTime
	CompletedAt     sql.NullTime
	OrderID         sql.NullInt64
	CreatedAt       time.Time
	UpdatedAt       time.Time
	Lines           []CheckoutLine
}

type CheckoutLine struct {
	ID             int
	CheckoutID     int
	ProductID      int
	ProductName    string
	Quantity       int
	UnitPrice      float64
	LineTotal      float64
	ReservationKey sql.NullString
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type CheckoutInput struct {
	Token          string
	Currency       string
	PaymentMethod  string
	SubtotalAmount float64
	ShippingFee    float64
	TaxAmount      float64
	TotalAmount    float64
	ExpiresAt      time.Time
	Lines          []CheckoutLineInput
}

type CheckoutLineInput struct {
	ProductID      int
	ProductName    string
	Quantity       int
	UnitPrice      float64
	LineTotal      float64
	ReservationKey string
}

const (
	PaymentStatusPending  = "pending"
	PaymentStatusCaptured = "captured"
	PaymentStatusFailed   = "failed"
	PaymentStatusRefunded = "refunded"
)

const (
	PaymentAttemptStatusPending   = "pending"
	PaymentAttemptStatusCompleted = "completed"
	PaymentAttemptStatusFailed    = "failed"
)

type Payment struct {
	ID                int
	OrderID           int
	UserID            int
	Method            string
	Provider          string
	Status            string
	Currency          string
	Amount            float64
	ExternalReference sql.NullString
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type PaymentAttempt struct {
	ID                int
	PaymentID         int
	Status            string
	RequestReference  sql.NullString
	ExternalReference sql.NullString
	ErrorMessage      sql.NullString
	CreatedAt         time.Time
}

type FulfillmentOrder struct {
	ID              int
	CustomerUserID  int
	CustomerName    string
	Status          string
	PartnerStatus   string
	DeliveryStatus  string
	DeliveryNotice  string
	DeliveryAddress string
	TotalAmount     float64
	CreatedAt       time.Time
	Items           []OrderItem
}

type PartnerOrderSummary struct {
	NewCount        int
	InProgressCount int
	DispatchedCount int
	CompletedCount  int
	OverdueCount    int
}

type AuditLog struct {
	ID          int
	ActorUserID sql.NullInt64
	Action      string
	TargetType  string
	TargetID    sql.NullInt64
	Details     string
	CreatedAt   time.Time
}

type CheckoutPlacement struct {
	OrderID     int64
	PaymentID   int64
	TotalAmount float64
	Reused      bool
}

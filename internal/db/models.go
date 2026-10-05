package db

import (
	"database/sql"
	"time"

	"github.com/kyambuthia/sokomoko/internal/money"
)

const (
	RoleUser  = "user"
	RoleStaff = "staff"
	RoleAdmin = "admin"
)

// User represents an account in the system. Passwords are bcrypt hashes; the
// bcrypt format embeds its own salt.
type User struct {
	ID           int
	Username     string
	Email        string
	PasswordHash string
	Role         string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    sql.NullTime
}

type Partner struct {
	ID           int
	Name         string
	Slug         string
	ContactEmail string
	Status       string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

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

// Product is a catalog listing. StockQuantity is the currently available
// quantity derived from inventory (on hand minus reserved and allocated).
type Product struct {
	ID            int
	Name          string
	Slug          string
	Description   string
	Price         money.Cents
	Currency      string
	StockQuantity int
	Category      string
	CategoryID    sql.NullInt64
	PartnerID     sql.NullInt64
	PartnerName   string
	Images        []ProductImage
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// PrimaryImage returns the raw URL of the first image, or "" when none exist.
func (p Product) PrimaryImage() string {
	if len(p.Images) == 0 {
		return ""
	}
	return p.Images[0].URL
}

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
}

type InventoryStock struct {
	ProductID         int
	WarehouseID       int
	OnHandQuantity    int
	ReservedQuantity  int
	AllocatedQuantity int
	AvailableQuantity int
	UpdatedAt         time.Time
}

const (
	StockReservationStatusActive    = "active"
	StockReservationStatusReleased  = "released"
	StockReservationStatusExpired   = "expired"
	StockReservationStatusConverted = "converted"
)

type StockReservation struct {
	ID             int
	ProductID      int
	WarehouseID    int
	UserID         int
	ReservationKey string
	Quantity       int
	Status         string
	ExpiresAt      time.Time
	ReleasedAt     sql.NullTime
	CreatedAt      time.Time
}

type StockMovement struct {
	ID            int
	ProductID     int
	WarehouseID   int
	MovementType  string
	QuantityDelta int
	Reference     string
	CreatedAt     time.Time
}

type Session struct {
	ID        string
	UserID    int
	CSRFToken string
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
	UnitPrice       money.Cents
	Quantity        int
	StockQuantity   int
	LineTotal       money.Cents
}

type OrderItem struct {
	ProductID   int
	PartnerID   sql.NullInt64
	ProductName string
	Quantity    int
	UnitPrice   money.Cents
	LineTotal   money.Cents
}

const (
	OrderStatusPending    = "pending"
	OrderStatusProcessing = "processing"
	OrderStatusShipped    = "shipped"
	OrderStatusDelivered  = "delivered"
	OrderStatusCancelled  = "cancelled"
)

// Order is the shared order projection used by customer, partner, and admin
// views.
type Order struct {
	ID              int
	UserID          int
	CustomerName    string
	Status          string
	PartnerStatus   string
	DeliveryStatus  string
	InventoryState  string
	DeliveryAddress string
	DeliveryNotice  string
	Currency        string
	Subtotal        money.Cents
	ShippingFee     money.Cents
	TaxAmount       money.Cents
	TotalAmount     money.Cents
	CreatedAt       time.Time
	UpdatedAt       time.Time
	Items           []OrderItem
}

// OrderTotals carries the priced breakdown for an order.
type OrderTotals struct {
	Subtotal    money.Cents
	ShippingFee money.Cents
	TaxAmount   money.Cents
	Total       money.Cents
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
	SubtotalAmount  money.Cents
	ShippingFee     money.Cents
	TaxAmount       money.Cents
	TotalAmount     money.Cents
	DeliveryAddress string
	ExpiresAt       time.Time
	CompletedAt     sql.NullTime
	OrderID         sql.NullInt64
	CreatedAt       time.Time
	UpdatedAt       time.Time
	Lines           []CheckoutLine
}

type CheckoutLine struct {
	ID          int
	CheckoutID  int
	ProductID   int
	ProductName string
	Quantity    int
	UnitPrice   money.Cents
	LineTotal   money.Cents
}

type CheckoutInput struct {
	Token          string
	Currency       string
	PaymentMethod  string
	SubtotalAmount money.Cents
	ShippingFee    money.Cents
	TaxAmount      money.Cents
	TotalAmount    money.Cents
	ExpiresAt      time.Time
	Lines          []CheckoutLineInput
}

type CheckoutLineInput struct {
	ProductID   int
	ProductName string
	Quantity    int
	UnitPrice   money.Cents
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
	Amount            money.Cents
	ExternalReference sql.NullString
	CreatedAt         time.Time
	UpdatedAt         time.Time
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
	ActorName   string
	Action      string
	TargetType  string
	TargetID    sql.NullInt64
	Details     string
	CreatedAt   time.Time
}

type CheckoutPlacement struct {
	OrderID     int64
	PaymentID   int64
	TotalAmount money.Cents
	Reused      bool
}

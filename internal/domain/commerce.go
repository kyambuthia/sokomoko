package domain

import (
	"errors"
	"time"

	"github.com/kyambuthia/sokomoko/internal/money"
)

var (
	ErrCheckoutNotFound         = errors.New("checkout not found")
	ErrPaymentNotFound          = errors.New("payment not found")
	ErrPaymentExceptionNotFound = errors.New("payment exception not found")
	ErrCurrencyLocked           = errors.New("currency cannot change after orders exist")
)

const (
	OrderStatusPending    = "pending"
	OrderStatusProcessing = "processing"
	OrderStatusShipped    = "shipped"
	OrderStatusDelivered  = "delivered"
	OrderStatusCancelled  = "cancelled"
)

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

// PaymentException is one confirmed charge that needs an operator refund.
type PaymentException struct {
	ID               int
	PaymentID        int
	AttemptID        int
	OrderID          int
	Reason           string
	Status           string
	Provider         string
	Currency         string
	Amount           money.Cents
	RequestReference string
	Receipt          string
	RefundReference  string
	LastError        string
	CreatedAt        time.Time
}

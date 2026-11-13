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

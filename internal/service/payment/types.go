package payment

import "github.com/kyambuthia/sokomoko/internal/db"

type PaymentException = db.PaymentException

var (
	ErrPaymentNotFound          = db.ErrPaymentNotFound
	ErrPaymentExceptionNotFound = db.ErrPaymentExceptionNotFound
	ErrCurrencyLocked           = db.ErrCurrencyLocked
)

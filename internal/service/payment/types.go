package payment

import "github.com/kyambuthia/sokomoko/internal/domain"

type PaymentException = domain.PaymentException

var (
	ErrPaymentNotFound          = domain.ErrPaymentNotFound
	ErrPaymentExceptionNotFound = domain.ErrPaymentExceptionNotFound
	ErrCurrencyLocked           = domain.ErrCurrencyLocked
)

package ui

import (
	"sync/atomic"

	"github.com/kyambuthia/sokomoko/internal/money"
)

// storeCurrency is the marketplace currency used to format prices. It is set
// at startup and whenever an admin changes it. The store runs as a single
// instance, so process-local state is enough.
var storeCurrency atomic.Value

func init() { storeCurrency.Store("USD") }

// SetCurrency changes the currency used by the money template function.
func SetCurrency(code string) {
	if _, ok := money.LookupCurrency(code); ok {
		storeCurrency.Store(code)
	}
}

// Currency returns the store currency code.
func Currency() string { return storeCurrency.Load().(string) }

// CurrencyMinorDigits returns the ISO 4217 minor-unit precision for the store.
func CurrencyMinorDigits() int {
	currency, ok := money.LookupCurrency(Currency())
	if !ok {
		return 2
	}
	return currency.MinorDigits
}

// FormatMoney renders an amount in the store currency, for example
// "KES 1,250.00".
func FormatMoney(c money.Cents) string { return money.Format(c, Currency()) }

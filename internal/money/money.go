// Package money represents currency amounts as integer minor units (cents).
//
// Floating point values are never used for stored or computed amounts so that
// totals, taxes, and aggregates stay exact.
package money

import (
	"errors"
	"strconv"
	"strings"
)

// Cents is an amount in minor currency units.
type Cents int64

var ErrInvalidAmount = errors.New("invalid amount")

// String renders the amount as a plain decimal string such as "12.50".
func (c Cents) String() string {
	sign := ""
	value := int64(c)
	if value < 0 {
		sign = "-"
		value = -value
	}
	whole := value / 100
	frac := value % 100
	fracStr := strconv.FormatInt(frac, 10)
	if frac < 10 {
		fracStr = "0" + fracStr
	}
	return sign + strconv.FormatInt(whole, 10) + "." + fracStr
}

// Float64 returns the amount in major units. It is intended for display and
// interoperability only; never use it for arithmetic on stored amounts.
func (c Cents) Float64() float64 {
	return float64(c) / 100
}

// Parse converts a user supplied decimal string ("12", "12.5", "12.50") into
// cents. Negative values, more than two fractional digits, and non-numeric
// input are rejected.
func Parse(raw string) (Cents, error) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "$")
	if s == "" {
		return 0, ErrInvalidAmount
	}

	wholePart, fracPart, hasFrac := strings.Cut(s, ".")
	if wholePart == "" {
		wholePart = "0"
	}
	if !isDigits(wholePart) || (hasFrac && (fracPart == "" || len(fracPart) > 2 || !isDigits(fracPart))) {
		return 0, ErrInvalidAmount
	}
	if len(wholePart) > 13 {
		return 0, ErrInvalidAmount
	}

	whole, err := strconv.ParseInt(wholePart, 10, 64)
	if err != nil {
		return 0, ErrInvalidAmount
	}
	var frac int64
	if hasFrac {
		if len(fracPart) == 1 {
			fracPart += "0"
		}
		frac, err = strconv.ParseInt(fracPart, 10, 64)
		if err != nil {
			return 0, ErrInvalidAmount
		}
	}
	return Cents(whole*100 + frac), nil
}

// MulBasisPoints multiplies the amount by rate/10000 rounding half away from
// zero. A rate of 800 is 8%.
func (c Cents) MulBasisPoints(rate int64) Cents {
	product := int64(c) * rate
	if product >= 0 {
		return Cents((product + 5000) / 10000)
	}
	return Cents((product - 5000) / 10000)
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

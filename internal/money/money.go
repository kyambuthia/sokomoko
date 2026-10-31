// Package money represents currency amounts as integer minor units (cents).
//
// Floating point values are never used for stored or computed amounts so that
// totals, taxes, and aggregates stay exact.
package money

import (
	"errors"
	"math/big"
	"strconv"
	"strings"
)

// Cents is an amount in minor currency units.
type Cents int64

var ErrInvalidAmount = errors.New("invalid amount")

// String renders the amount as a plain decimal string such as "12.50".
func (c Cents) String() string {
	value := int64(c)
	sign := ""
	var magnitude uint64
	if value < 0 {
		sign = "-"
		magnitude = uint64(-(value + 1)) + 1
	} else {
		magnitude = uint64(value)
	}
	whole := magnitude / 100
	frac := magnitude % 100
	fracStr := strconv.FormatUint(frac, 10)
	if frac < 10 {
		fracStr = "0" + fracStr
	}
	return sign + strconv.FormatUint(whole, 10) + "." + fracStr
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
// zero. A rate of 800 is 8%. Results outside the Cents range saturate.
func (c Cents) MulBasisPoints(rate int64) Cents {
	var product, multiplier, scale, half, quotient, remainder big.Int
	product.SetInt64(int64(c))
	multiplier.SetInt64(rate)
	product.Mul(&product, &multiplier)
	scale.SetInt64(10_000)
	half.SetInt64(5_000)
	quotient.QuoRem(&product, &scale, &remainder)
	if remainder.Abs(&remainder).Cmp(&half) >= 0 {
		var one big.Int
		one.SetInt64(1)
		if product.Sign() < 0 {
			quotient.Sub(&quotient, &one)
		} else {
			quotient.Add(&quotient, &one)
		}
	}
	if !quotient.IsInt64() {
		if quotient.Sign() < 0 {
			return Cents(-1 << 63)
		}
		return Cents(1<<63 - 1)
	}
	return Cents(quotient.Int64())
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

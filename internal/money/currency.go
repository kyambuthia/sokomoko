package money

import "strings"

// Currency describes how a store currency is shown to customers. All
// supported currencies use at most two minor-unit digits, matching Cents.
type Currency struct {
	Code        string // ISO 4217, for example "KES"
	Name        string
	Prefix      string // printed before the amount, for example "$" or "KES "
	MinorDigits int    // ISO 4217 minor-unit digits
}

// Currencies lists the store currencies an admin can pick. Payment providers
// declare which of these they can charge.
var Currencies = []Currency{
	{Code: "USD", Name: "US dollar", Prefix: "$", MinorDigits: 2},
	{Code: "KES", Name: "Kenyan shilling", Prefix: "KES ", MinorDigits: 2},
	{Code: "NGN", Name: "Nigerian naira", Prefix: "₦", MinorDigits: 2},
	{Code: "GHS", Name: "Ghanaian cedi", Prefix: "GH₵", MinorDigits: 2},
	{Code: "ZAR", Name: "South African rand", Prefix: "R ", MinorDigits: 2},
	{Code: "UGX", Name: "Ugandan shilling", Prefix: "UGX ", MinorDigits: 0},
	{Code: "TZS", Name: "Tanzanian shilling", Prefix: "TZS ", MinorDigits: 2},
	{Code: "RWF", Name: "Rwandan franc", Prefix: "RWF ", MinorDigits: 0},
	{Code: "EUR", Name: "Euro", Prefix: "€", MinorDigits: 2},
	{Code: "GBP", Name: "Pound sterling", Prefix: "£", MinorDigits: 2},
}

// LookupCurrency returns the currency for code and whether it is supported.
func LookupCurrency(code string) (Currency, bool) {
	code = strings.ToUpper(strings.TrimSpace(code))
	for _, c := range Currencies {
		if c.Code == code {
			return c, true
		}
	}
	return Currency{}, false
}

// InputString renders c as an ungrouped decimal suitable for a money input.
// Whole amounts in zero-digit currencies omit the decimal suffix. Existing
// fractional values remain visible so legacy records are never rounded away.
func InputString(c Cents, code string) string {
	plain := c.String()
	if currency, ok := LookupCurrency(code); ok && currency.MinorDigits == 0 {
		plain = strings.TrimSuffix(strings.TrimRight(plain, "0"), ".")
	}
	return plain
}

// Format renders c with the currency prefix and thousands separators, for
// example "KES 1,250.00". Zero-digit currencies omit .00 for whole amounts.
// Unknown codes fall back to "XYZ 1,250.00".
func Format(c Cents, code string) string {
	currency, ok := LookupCurrency(code)
	if !ok {
		currency = Currency{Code: code, Prefix: strings.ToUpper(code) + " ", MinorDigits: 2}
	}
	plain := InputString(c, code)
	sign := ""
	if strings.HasPrefix(plain, "-") {
		sign = "-"
		plain = strings.TrimPrefix(plain, "-")
	}
	whole, frac, hasFrac := strings.Cut(plain, ".")
	result := sign + currency.Prefix + groupThousands(whole)
	if hasFrac {
		result += "." + frac
	}
	return result
}

func groupThousands(digits string) string {
	if len(digits) <= 3 {
		return digits
	}
	var b strings.Builder
	lead := len(digits) % 3
	if lead > 0 {
		b.WriteString(digits[:lead])
	}
	for i := lead; i < len(digits); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(digits[i : i+3])
	}
	return b.String()
}

// WholeUnitsCeil rounds up to whole major units, for providers such as
// M-Pesa that only charge whole shillings.
func (c Cents) WholeUnitsCeil() int64 {
	if c <= 0 {
		return 0
	}
	amount := int64(c)
	whole := amount / 100
	if amount%100 != 0 {
		whole++
	}
	return whole
}

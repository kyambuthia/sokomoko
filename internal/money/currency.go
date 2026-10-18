package money

import "strings"

// Currency describes how a store currency is shown to customers. All
// supported currencies use two minor-unit digits, matching Cents.
type Currency struct {
	Code   string // ISO 4217, for example "KES"
	Name   string
	Prefix string // printed before the amount, for example "$" or "KES "
}

// Currencies lists the store currencies an admin can pick. Payment providers
// declare which of these they can charge.
var Currencies = []Currency{
	{Code: "USD", Name: "US dollar", Prefix: "$"},
	{Code: "KES", Name: "Kenyan shilling", Prefix: "KES "},
	{Code: "NGN", Name: "Nigerian naira", Prefix: "₦"},
	{Code: "GHS", Name: "Ghanaian cedi", Prefix: "GH₵"},
	{Code: "ZAR", Name: "South African rand", Prefix: "R "},
	{Code: "UGX", Name: "Ugandan shilling", Prefix: "UGX "},
	{Code: "TZS", Name: "Tanzanian shilling", Prefix: "TZS "},
	{Code: "RWF", Name: "Rwandan franc", Prefix: "RWF "},
	{Code: "EUR", Name: "Euro", Prefix: "€"},
	{Code: "GBP", Name: "Pound sterling", Prefix: "£"},
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

// Format renders c with the currency prefix and thousands separators, for
// example "KES 1,250.00". Unknown codes fall back to "XYZ 1,250.00".
func Format(c Cents, code string) string {
	currency, ok := LookupCurrency(code)
	if !ok {
		currency = Currency{Code: code, Prefix: strings.ToUpper(code) + " "}
	}
	sign := ""
	if c < 0 {
		sign = "-"
		c = -c
	}
	plain := c.String()
	whole, frac, _ := strings.Cut(plain, ".")
	return sign + currency.Prefix + groupThousands(whole) + "." + frac
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
	return (int64(c) + 99) / 100
}

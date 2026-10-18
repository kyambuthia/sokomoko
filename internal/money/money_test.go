package money

import "testing"

func TestCentsString(t *testing.T) {
	cases := map[Cents]string{
		0:      "0.00",
		5:      "0.05",
		1250:   "12.50",
		99999:  "999.99",
		-1205:  "-12.05",
		100000: "1000.00",
	}
	for value, want := range cases {
		if got := value.String(); got != want {
			t.Errorf("Cents(%d).String() = %q, want %q", int64(value), got, want)
		}
	}
}

func TestParse(t *testing.T) {
	valid := map[string]Cents{
		"12":     1200,
		"12.5":   1250,
		"12.50":  1250,
		" 0.99 ": 99,
		".75":    75,
		"$3.10":  310,
	}
	for raw, want := range valid {
		got, err := Parse(raw)
		if err != nil {
			t.Errorf("Parse(%q) error = %v", raw, err)
			continue
		}
		if got != want {
			t.Errorf("Parse(%q) = %d, want %d", raw, got, want)
		}
	}

	for _, raw := range []string{"", "-1", "1.234", "abc", "1.", "1e3", "12,50", "99999999999999"} {
		if _, err := Parse(raw); err == nil {
			t.Errorf("Parse(%q) expected error", raw)
		}
	}
}

func TestMulBasisPoints(t *testing.T) {
	cases := []struct {
		amount Cents
		rate   int64
		want   Cents
	}{
		{1000, 800, 80},
		{1999, 800, 160}, // 159.92 rounds to 160
		{1006, 800, 80},  // 80.48 rounds to 80
		{1007, 800, 81},  // 80.56 rounds to 81
		{0, 800, 0},
	}
	for _, tc := range cases {
		if got := tc.amount.MulBasisPoints(tc.rate); got != tc.want {
			t.Errorf("%d * %d bp = %d, want %d", tc.amount, tc.rate, got, tc.want)
		}
	}
}

func TestFormat(t *testing.T) {
	cases := []struct {
		amount Cents
		code   string
		want   string
	}{
		{1250, "USD", "$12.50"},
		{125000, "KES", "KES 1,250.00"},
		{123456789, "NGN", "₦1,234,567.89"},
		{5, "ZAR", "R 0.05"},
		{-100000, "GBP", "-£1,000.00"},
		{100, "xyz", "XYZ 1.00"},
	}
	for _, tc := range cases {
		if got := Format(tc.amount, tc.code); got != tc.want {
			t.Errorf("Format(%d, %q) = %q, want %q", tc.amount, tc.code, got, tc.want)
		}
	}
}

func TestWholeUnitsCeil(t *testing.T) {
	for in, want := range map[Cents]int64{0: 0, 1: 1, 100: 1, 101: 2, 125000: 1250, -5: 0} {
		if got := in.WholeUnitsCeil(); got != want {
			t.Errorf("%d.WholeUnitsCeil() = %d, want %d", in, got, want)
		}
	}
}

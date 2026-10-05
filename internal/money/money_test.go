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

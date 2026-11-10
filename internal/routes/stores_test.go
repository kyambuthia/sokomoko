package routes

import "testing"

func TestStoreInitials(t *testing.T) {
	cases := []struct{ name, want string }{
		{"", "S"},
		{"Urban", "U"},
		{"Urban Goods", "UG"},
		{"  nature supply  ", "NS"},
		{"École Shop", "ÉS"},
	}
	for _, tc := range cases {
		if got := storeInitials(tc.name); got != tc.want {
			t.Fatalf("storeInitials(%q)=%q, want %q", tc.name, got, tc.want)
		}
	}
}

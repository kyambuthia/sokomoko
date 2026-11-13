package netutil

import "testing"

func TestNormalizeRateLimitKey(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "exact IPv4",
			raw:  "192.168.1.42",
			want: "192.168.1.42",
		},
		{
			name: "IPv4 with surrounding whitespace",
			raw:  "  10.0.0.1 \t\n",
			want: "10.0.0.1",
		},
		{
			name: "IPv4 mapped into IPv6 unmapped to IPv4",
			raw:  "::ffff:192.0.2.1",
			want: "192.0.2.1",
		},
		{
			name: "native IPv6 masked to /64",
			raw:  "2001:db8:abcd:12:34:56:78:90",
			want: "2001:db8:abcd:12::",
		},
		{
			name: "another host in the same IPv6 /64 prefix",
			raw:  "2001:db8:abcd:12:ffff:ffff:ffff:ffff",
			want: "2001:db8:abcd:12::",
		},
		{
			name: "scoped IPv6 zone stripped before masking",
			raw:  "fe80::1ff:fe00:1%eth0",
			want: "fe80::",
		},
		{
			name: "non-IP fallback trimmed",
			raw:  "   user:12345   ",
			want: "user:12345",
		},
		{
			name: "empty string",
			raw:  "   ",
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizeRateLimitKey(tc.raw)
			if got != tc.want {
				t.Fatalf("NormalizeRateLimitKey(%q) = %q; want %q", tc.raw, got, tc.want)
			}
		})
	}
}

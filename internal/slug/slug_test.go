package slug

import (
	"strings"
	"testing"
)

func TestMake(t *testing.T) {
	cases := map[string]string{
		"  Quiet Paper / Desk Lamp  ": "quiet-paper-desk-lamp",
		"Café Crème 2000":             "caf-cr-me-2000",
		"---":                         "",
		"Already-slugged":             "already-slugged",
	}
	for in, want := range cases {
		if got := Make(in); got != want {
			t.Errorf("Make(%q) = %q, want %q", in, got, want)
		}
	}
	if got := Make(strings.Repeat("ab ", 60)); len(got) > MaxLength || strings.HasSuffix(got, "-") {
		t.Errorf("long input produced %q", got)
	}
}

func TestValid(t *testing.T) {
	for s, want := range map[string]bool{
		"good": true, "good-slug-2": true, "Bad": false, "bad slug": false,
		"-lead": false, "double--dash": false, "": false, strings.Repeat("a", MaxLength+1): false,
	} {
		if got := Valid(s); got != want {
			t.Errorf("Valid(%q) = %v, want %v", s, got, want)
		}
	}
}

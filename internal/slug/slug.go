// Package slug builds and validates the URL slugs used for products,
// categories, partners, and the store.
package slug

import (
	"regexp"
	"strings"
)

// MaxLength caps generated slugs.
const MaxLength = 80

var pattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// Make lowercases value and joins its ASCII letter and digit runs with single
// dashes, truncated to MaxLength. It returns "" when nothing usable remains.
func Make(value string) string {
	var b strings.Builder
	pendingDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		isAlnum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if !isAlnum {
			pendingDash = b.Len() > 0
			continue
		}
		if pendingDash {
			b.WriteByte('-')
			pendingDash = false
		}
		b.WriteRune(r)
		if b.Len() >= MaxLength {
			break
		}
	}
	return strings.TrimRight(b.String(), "-")
}

// Valid reports whether s is lowercase letters and digits separated by single
// dashes, at most MaxLength bytes. This matches the database CHECK constraints
// for product, category, and store slugs.
func Valid(s string) bool {
	return len(s) <= MaxLength && pattern.MatchString(s)
}

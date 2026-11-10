package ui

import (
	"strings"
	"unicode"
)

// serviceCategoryIcons pairs each directory category with a sprite icon.
// Unknown categories fall back to the generic tag icon.
var serviceCategoryIcons = map[string]string{
	"all":      "grid",
	"hair":     "scissors",
	"beauty":   "sparkles",
	"dental":   "dental",
	"wellness": "massage",
	"home":     "tools",
	"other":    "category",
}

// ServiceIcon returns the sprite icon name for a service category.
func ServiceIcon(category string) string {
	if icon, ok := serviceCategoryIcons[category]; ok {
		return icon
	}
	return "category"
}

// ServiceModeLabel describes where a service happens in buyer terms.
func ServiceModeLabel(mode string) string {
	switch mode {
	case "premises":
		return "At the business"
	case "mobile":
		return "Provider travels to you"
	default:
		return "Remote"
	}
}

// ServiceModeIcon is the icon that accompanies ServiceModeLabel.
func ServiceModeIcon(mode string) string {
	switch mode {
	case "premises":
		return "store"
	case "mobile":
		return "walk"
	default:
		return "world"
	}
}

// Initials returns up to two uppercase letters for a monochrome avatar:
// the first letter of the first two words, ignoring a leading "@".
func Initials(name string) string {
	var out []rune
	for _, word := range strings.Fields(strings.TrimPrefix(strings.TrimSpace(name), "@")) {
		for _, r := range word {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				out = append(out, unicode.ToUpper(r))
				break
			}
		}
		if len(out) == 2 {
			break
		}
	}
	if len(out) == 0 {
		return "?"
	}
	return string(out)
}

package ui

import "strings"

// categoryIconExact pins the category slugs this store seeds, so their pill
// does not depend on which keyword happens to match first.
var categoryIconExact = map[string]string{
	"electronics":    "mobile",
	"lighting-audio": "headphones",
	"home-living":    "home",
	"accessories":    "backpack",
}

// categoryIconRules pairs a slug token prefix with a sprite icon. Admins create
// categories freely, so a pill is matched on its slug words and anything
// unrecognised falls back to the generic tag rather than rendering bare.
var categoryIconRules = []struct{ keyword, icon string }{
	{"audio", "headphones"},
	{"headphone", "headphones"},
	{"speaker", "music"},
	{"music", "music"},
	{"instrument", "music"},

	{"light", "lamp"},
	{"lamp", "lamp"},

	{"electronic", "mobile"},
	{"gadget", "mobile"},
	{"phone", "mobile"},
	{"smartphone", "mobile"},
	{"tablet", "laptop"},
	{"computer", "laptop"},
	{"laptop", "laptop"},
	{"camera", "camera"},
	{"photo", "camera"},

	{"gaming", "gamepad"},
	{"game", "gamepad"},
	{"console", "gamepad"},

	{"home", "home"},
	{"living", "home"},
	{"furniture", "bed"},
	{"bedding", "bed"},
	{"mattress", "bed"},

	{"kitchen", "kitchen"},
	{"cookware", "kitchen"},
	{"food", "coffee"},
	{"grocery", "apple"},
	{"drink", "coffee"},
	{"coffee", "coffee"},

	{"cloth", "shirt"},
	{"apparel", "shirt"},
	{"fashion", "shirt"},
	{"shirt", "shirt"},
	{"activewear", "shirt"},
	{"shoe", "shoe"},
	{"footwear", "shoe"},
	{"sneaker", "shoe"},

	{"accessor", "backpack"},
	{"bag", "backpack"},
	{"jewel", "diamond"},
	{"watch", "watch"},
	{"glasses", "glasses"},
	{"eyewear", "glasses"},

	{"beauty", "perfume"},
	{"cosmetic", "perfume"},
	{"skincare", "perfume"},
	{"perfume", "perfume"},
	{"health", "heart"},
	{"pharmacy", "heart"},
	{"supplement", "heart"},

	{"sport", "dumbbell"},
	{"fitness", "dumbbell"},
	{"gym", "dumbbell"},

	{"bike", "bike"},
	{"cycle", "bike"},
	{"car", "car"},
	{"auto", "car"},
	{"motor", "car"},

	{"book", "book"},
	{"stationery", "brush"},
	{"office", "book"},

	{"art", "palette"},
	{"craft", "palette"},
	{"paint", "palette"},

	{"toy", "basketball"},
	{"baby", "baby"},
	{"kid", "baby"},
	{"child", "baby"},
	{"pet", "paw"},
	{"animal", "paw"},
	{"fish", "fish"},

	{"garden", "plant"},
	{"plant", "plant"},
	{"flower", "plant"},

	{"tool", "tools"},
	{"diy", "tools"},
	{"hardware", "tools"},

	{"gift", "gift"},
}

// CategoryIcon returns the sprite icon for a category slug. Each hyphen-separated
// word is matched by prefix, so "shoes" and "shoe" both reach the shoe icon.
func CategoryIcon(slug string) string {
	s := strings.ToLower(strings.TrimSpace(slug))
	if icon, ok := categoryIconExact[s]; ok {
		return icon
	}
	for _, token := range strings.Split(s, "-") {
		for _, rule := range categoryIconRules {
			if strings.HasPrefix(token, rule.keyword) {
				return rule.icon
			}
		}
	}
	return "category"
}

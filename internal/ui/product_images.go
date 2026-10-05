package ui

import "strings"

// DefaultProductImage is used when a product has no usable image at all.
const DefaultProductImage = "images/gray_fabric_bluetooth_speaker.png"

var bundledProductImages = []string{
	"images/black_adjustable_desk_lamp.png",
	"images/black_over_ear_headphones.png",
	"images/gray_fabric_bluetooth_speaker.png",
	"images/iphone_category_card.png",
	"images/product_tv.png",
	"images/stainless_steel_electric_kettle.png",
	"images/white_wireless_earbuds_charging_case.png",
}

// ProductImageURL normalizes a stored product image URL into a cache-busted
// local static asset URL. Remote URLs are not hot-linked (the CSP only allows
// same-origin images), so they fall back to a bundled placeholder chosen from
// the product's slug, name, and category.
func ProductImageURL(rawURL, slug, name, category string) string {
	clean := strings.TrimSpace(rawURL)
	switch {
	case strings.HasPrefix(clean, "data:image/"):
		return clean
	case clean == "", strings.HasPrefix(clean, "http://"), strings.HasPrefix(clean, "https://"), strings.HasPrefix(clean, "//"):
		return AssetURL(fallbackProductImage(slug, name, category))
	}

	pathPart := clean
	if idx := strings.IndexAny(pathPart, "?#"); idx >= 0 {
		pathPart = pathPart[:idx]
	}
	if strings.Contains(pathPart, "..") {
		return AssetURL(fallbackProductImage(slug, name, category))
	}
	return AssetURL(pathPart)
}

func fallbackProductImage(slug, name, category string) string {
	descriptor := strings.ToLower(strings.TrimSpace(strings.Join([]string{slug, name, category}, " ")))
	switch {
	case descriptor == "":
		return DefaultProductImage
	case strings.Contains(descriptor, "earbud"):
		return "images/white_wireless_earbuds_charging_case.png"
	case strings.Contains(descriptor, "headphone"):
		return "images/black_over_ear_headphones.png"
	case strings.Contains(descriptor, "speaker"):
		return "images/gray_fabric_bluetooth_speaker.png"
	case strings.Contains(descriptor, "lamp"):
		return "images/black_adjustable_desk_lamp.png"
	case strings.Contains(descriptor, "kettle"):
		return "images/stainless_steel_electric_kettle.png"
	case strings.Contains(descriptor, "tv"), strings.Contains(descriptor, "television"), strings.Contains(descriptor, "monitor"):
		return "images/product_tv.png"
	}

	sum := 0
	for _, r := range descriptor {
		sum += int(r)
	}
	return bundledProductImages[sum%len(bundledProductImages)]
}

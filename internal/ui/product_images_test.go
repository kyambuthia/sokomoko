package ui

import (
	"strings"
	"testing"
)

func TestProductImageURL_ExternalURLFallsBackToBundledAsset(t *testing.T) {
	got := ProductImageURL("https://picsum.photos/seed/earbuds/400/400", "wireless-earbuds-pro", "Wireless Earbuds Pro", "Audio")
	if !strings.HasPrefix(got, "/static/images/white_wireless_earbuds_charging_case.png?v=") {
		t.Fatalf("expected bundled earbuds image, got %q", got)
	}
}

func TestProductImageURL_LocalStaticPathPassesThrough(t *testing.T) {
	got := ProductImageURL("/static/images/product_tv.png", "living-room-display", "Living Room Display", "Electronics")
	if !strings.HasPrefix(got, "/static/images/product_tv.png?v=") {
		t.Fatalf("expected cache-busted static image URL, got %q", got)
	}
}

func TestProductImageURL_UsesFallbackWhenImageMissing(t *testing.T) {
	got := ProductImageURL("", "led-desk-lamp", "LED Desk Lamp", "Lighting")
	if !strings.HasPrefix(got, "/static/images/black_adjustable_desk_lamp.png?v=") {
		t.Fatalf("expected bundled lamp image, got %q", got)
	}
}

func TestProductImageURL_RejectsTraversal(t *testing.T) {
	got := ProductImageURL("/static/../../etc/passwd", "kettle", "Kettle", "")
	if !strings.HasPrefix(got, "/static/images/stainless_steel_electric_kettle.png") {
		t.Fatalf("expected fallback for traversal path, got %q", got)
	}
}

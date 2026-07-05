package db

import (
	"strings"
	"testing"
)

func TestResolveProductImageURL_ExternalURLFallsBackToBundledAsset(t *testing.T) {
	got := ResolveProductImageURL("https://picsum.photos/seed/earbuds/400/400", "wireless-earbuds-pro", "Wireless Earbuds Pro", "Audio")

	if !strings.HasPrefix(got, "/static/images/white_wireless_earbuds_charging_case.png?v=") {
		t.Fatalf("expected bundled earbuds image, got %q", got)
	}
}

func TestResolveProductImageURL_LocalStaticPathPassesThrough(t *testing.T) {
	got := ResolveProductImageURL("/static/images/product_tv.png", "living-room-display", "Living Room Display", "Electronics")

	if !strings.HasPrefix(got, "/static/images/product_tv.png?v=") {
		t.Fatalf("expected cache-busted static image URL, got %q", got)
	}
}

func TestProductGetPrimaryImageURL_UsesFallbackWhenImageMissing(t *testing.T) {
	product := Product{
		Slug:     "led-desk-lamp",
		Name:     "LED Desk Lamp",
		Category: "Lighting",
	}

	got := product.GetPrimaryImageURL()
	if !strings.HasPrefix(got, "/static/images/black_adjustable_desk_lamp.png?v=") {
		t.Fatalf("expected bundled lamp image, got %q", got)
	}
}

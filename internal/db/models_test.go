package db

import "testing"

func TestResolveProductImageURL_ExternalURLFallsBackToBundledAsset(t *testing.T) {
	got := ResolveProductImageURL("https://picsum.photos/seed/earbuds/400/400", "wireless-earbuds-pro", "Wireless Earbuds Pro", "Audio")

	if got != "/static/images/white_wireless_earbuds_charging_case.png" {
		t.Fatalf("expected bundled earbuds image, got %q", got)
	}
}

func TestResolveProductImageURL_LocalStaticPathPassesThrough(t *testing.T) {
	want := "/static/images/product_tv.png"
	got := ResolveProductImageURL(want, "living-room-display", "Living Room Display", "Electronics")

	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestProductGetPrimaryImageURL_UsesFallbackWhenImageMissing(t *testing.T) {
	product := Product{
		Slug:     "led-desk-lamp",
		Name:     "LED Desk Lamp",
		Category: "Lighting",
	}

	got := product.GetPrimaryImageURL()
	if got != "/static/images/black_adjustable_desk_lamp.png" {
		t.Fatalf("expected bundled lamp image, got %q", got)
	}
}

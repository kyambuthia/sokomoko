package db

import "testing"

func TestBrowseServices_RadiusAndPrivacy(t *testing.T) {
	s := newTestStore(t)
	_, owner, _ := chatShop(t, s)

	// Westlands premises listing
	westLat, westLng := -1.2647, 36.8019
	west := serviceFixture("westlands-salon")
	west.State = "published"
	west.DeliveryMode = "premises"
	west.PublicArea = "Westlands"
	west.PublicLat = &westLat
	west.PublicLng = &westLng
	_, err := s.SaveService(t.Context(), owner, west)
	chatMust(t, err)

	// Karen premises listing (~12km away)
	karenLat, karenLng := -1.3192, 36.7069
	karen := serviceFixture("karen-salon")
	karen.State = "published"
	karen.DeliveryMode = "premises"
	karen.PublicArea = "Karen"
	karen.PublicLat = &karenLat
	karen.PublicLng = &karenLng
	_, err = s.SaveService(t.Context(), owner, karen)
	chatMust(t, err)

	// Mobile provider in Westlands: coordinates must never be stored or exposed.
	mobile := serviceFixture("westlands-mobile")
	mobile.State = "published"
	mobile.DeliveryMode = "mobile"
	mobile.PublicArea = "Westlands"
	mobile.PublicLat = &westLat
	mobile.PublicLng = &westLng
	savedMobile, err := s.SaveService(t.Context(), owner, mobile)
	chatMust(t, err)
	if savedMobile.PublicLat != nil || savedMobile.PublicLng != nil {
		t.Fatalf("mobile service leaked coordinates: (%v, %v)", savedMobile.PublicLat, savedMobile.PublicLng)
	}

	// Radius 5km around Westlands returns only Westlands premises with DistanceKM populated.
	near, total, err := s.BrowseServices(t.Context(), ServiceFilter{
		Lat:      &westLat,
		Lng:      &westLng,
		RadiusKM: 5,
		Limit:    20,
	})
	chatMust(t, err)
	if total != 1 || len(near) != 1 || near[0].Slug != "westlands-salon" {
		t.Fatalf("5km browse = %+v (total %d); want only westlands-salon", near, total)
	}
	if near[0].DistanceKM == nil || *near[0].DistanceKM > 0.1 {
		t.Fatalf("expected distance ~0km, got %v", near[0].DistanceKM)
	}

	// Area text filter still finds both premises and mobile listings in Westlands.
	byArea, areaTotal, err := s.BrowseServices(t.Context(), ServiceFilter{
		Area:  "Westlands",
		Limit: 20,
	})
	chatMust(t, err)
	if areaTotal != 2 || len(byArea) != 2 {
		t.Fatalf("area browse = %+v (total %d); want 2", byArea, areaTotal)
	}
}

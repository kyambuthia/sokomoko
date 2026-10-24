package main

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/kyambuthia/sokomoko/internal/db"
	directorysvc "github.com/kyambuthia/sokomoko/internal/service/directory"
)

func TestServiceAPIEnforcesOwnershipPrivacyAndVersions(t *testing.T) {
	clearAllTables()
	createTestUser(t, "service-admin", "service-admin@example.com", "MessagingFixture123!", db.RoleAdmin)
	createTestUser(t, "service-seller", "service-seller@example.com", "MessagingFixture123!", db.RolePartner)
	createTestUser(t, "service-other", "service-other@example.com", "MessagingFixture123!", db.RolePartner)
	_, buyer := messageTestSession(t, "service-buyer", db.RoleUser, "localhost")
	createAPITestPartner(t, "service-seller")
	createAPITestPartner(t, "service-other")
	seller := []*http.Cookie{loginAndGetSessionCookie(t, "partner.localhost", "service-seller", "MessagingFixture123!")}
	other := []*http.Cookie{loginAndGetSessionCookie(t, "partner.localhost", "service-other", "MessagingFixture123!")}
	input := directorysvc.Input{Title: "Neighbourhood haircut", Description: "A sample haircut service for the demo directory.", Category: "hair", DurationMinutes: 30, PriceMode: "fixed", Price: "500.00", Currency: "KES", PublicArea: "Westlands", DeliveryMode: "premises", State: "draft"}
	request := func(method, path string, body any, who string, csrf bool, status int) []byte {
		t.Helper()
		cookies := seller
		if who == "other" {
			cookies = other
		}
		if who == "buyer" {
			cookies = buyer
		}
		if who == "anonymous" {
			cookies = nil
		}
		host := "partner.localhost"
		if who == "buyer" || who == "anonymous" {
			host = "localhost"
		}
		response, data := makeAPIRequestOnHost(t, host, method, path, body, cookies, csrf, nil)
		if response.StatusCode != status {
			t.Fatalf("%s %s: status=%d want=%d body=%s", method, path, response.StatusCode, status, data)
		}
		return data
	}
	const collection = "/api/v1/partner/services"
	request(http.MethodPost, collection, input, "buyer", true, http.StatusForbidden)
	request(http.MethodPost, collection, input, "seller", false, http.StatusForbidden)
	var listing db.ServiceListing
	decodeAPIData(t, request(http.MethodPost, collection, input, "seller", true, http.StatusCreated), &listing)
	if listing.Version != 1 || listing.State != "draft" {
		t.Fatalf("unexpected listing: %+v", listing)
	}
	item := fmt.Sprintf("%s/%d", collection, listing.ID)
	public := "/api/v1/catalog/services/" + listing.Slug
	request(http.MethodGet, public, nil, "anonymous", false, http.StatusNotFound)
	request(http.MethodGet, item, nil, "other", false, http.StatusNotFound)
	input.Version = listing.Version
	input.State = "published"
	request(http.MethodPatch, item, input, "other", true, http.StatusNotFound)
	decodeAPIData(t, request(http.MethodPatch, item, input, "seller", true, http.StatusOK), &listing)
	if listing.Version != 2 {
		t.Fatalf("version=%d", listing.Version)
	}
	request(http.MethodPatch, item, input, "seller", true, http.StatusConflict)
	request(http.MethodGet, public, nil, "anonymous", false, http.StatusOK)
	var page struct {
		Items []db.ServiceListing `json:"items"`
		Total int                 `json:"total"`
	}
	decodeAPIData(t, request(http.MethodGet, "/api/v1/catalog/services?category=hair&area=westlands&q=haircut", nil, "anonymous", false, http.StatusOK), &page)
	if page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("unexpected results: %+v", page)
	}
	decodeAPIData(t, request(http.MethodGet, "/api/v1/catalog/services?q=%25", nil, "anonymous", false, http.StatusOK), &page)
	if page.Total != 0 {
		t.Fatal("wildcard expanded in literal search")
	}
	request(http.MethodPost, collection, map[string]any{"partner_id": 123}, "seller", true, http.StatusBadRequest)
	input.Version = listing.Version
	input.State = "archived"
	request(http.MethodPatch, item, input, "seller", true, http.StatusOK)
	request(http.MethodGet, public, nil, "anonymous", false, http.StatusNotFound)
}

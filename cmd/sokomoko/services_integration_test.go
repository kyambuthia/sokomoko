package main

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
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
	detailResponse, detailHTML := makeRequest(http.MethodGet, "/services/"+listing.Slug, nil, buyer, "localhost")
	enquiryPath := fmt.Sprintf("/messages/new?service_id=%d", listing.ID)
	if detailResponse.StatusCode != http.StatusOK || !strings.Contains(detailHTML, enquiryPath) {
		t.Fatal("service enquiry entry missing")
	}
	composeResponse, composeHTML := makeRequest(http.MethodGet, enquiryPath, nil, buyer, "localhost")
	if composeResponse.StatusCode != http.StatusOK || !strings.Contains(composeHTML, `name="service_id"`) || !strings.Contains(composeHTML, "Open encrypted conversation") || !strings.Contains(composeHTML, "Service · Neighbourhood haircut") {
		t.Fatalf("service compose status=%d body=%s", composeResponse.StatusCode, composeHTML)
	}
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
	// HTML forms use the same authorization, validation and version checks.
	editPath := fmt.Sprintf("/services/%d/edit", listing.ID)
	response, pageHTML := makeRequest(http.MethodGet, editPath, nil, seller, "partner.localhost")
	if response.StatusCode != http.StatusOK || !strings.Contains(pageHTML, `name="version" value="3"`) {
		t.Fatalf("edit page status=%d body=%s", response.StatusCode, pageHTML)
	}
	session, err := testStore.GetSession(t.Context(), seller[0].Value)
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{"csrf_token": {session.CSRFToken}, "version": {"3"}, "title": {"<script>alert(1)</script>"}, "description": {input.Description}, "category": {"hair"}, "duration_minutes": {"30"}, "price_mode": {"fixed"}, "price": {"500.00"}, "currency": {"KES"}, "public_area": {"Westlands"}, "delivery_mode": {"premises"}, "state": {"published"}}
	response, pageHTML = makeRequest(http.MethodPost, editPath, form, seller, "partner.localhost")
	if response.StatusCode != http.StatusSeeOther {
		t.Fatalf("save status=%d body=%s", response.StatusCode, pageHTML)
	}
	response, pageHTML = makeRequest(http.MethodGet, "/services/"+listing.Slug, nil, nil, "localhost")
	if response.StatusCode != http.StatusOK || strings.Contains(pageHTML, "<script>alert(1)</script>") || !strings.Contains(pageHTML, "&lt;script&gt;") {
		t.Fatalf("unsafe or missing service page status=%d", response.StatusCode)
	}
	response, pageHTML = makeRequest(http.MethodPost, editPath, form, seller, "partner.localhost")
	if response.StatusCode != http.StatusConflict || !strings.Contains(pageHTML, "Your text is kept below") {
		t.Fatalf("stale form status=%d body=%s", response.StatusCode, pageHTML)
	}
	response, pageHTML = makeRequest(http.MethodGet, "/services?area=Westlands", nil, nil, "localhost")
	if response.StatusCode != http.StatusOK || !strings.Contains(pageHTML, listing.Slug) {
		t.Fatalf("directory status=%d body=%s", response.StatusCode, pageHTML)
	}
	response, _ = makeRequest(http.MethodGet, editPath, nil, other, "partner.localhost")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-shop HTML status=%d", response.StatusCode)
	}
}

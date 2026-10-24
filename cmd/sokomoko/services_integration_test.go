package main

import (
	"encoding/json"
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
	decodeAPIData(t, request(http.MethodGet, "/api/v1/catalog/services?delivery_mode=mobile", nil, "anonymous", false, http.StatusOK), &page)
	if page.Total != 0 || len(page.Items) != 0 {
		t.Fatal("location type filter included a premises service")
	}
	request(http.MethodGet, "/api/v1/catalog/services?delivery_mode=unknown", nil, "anonymous", false, http.StatusBadRequest)
	modeResponse, modeHTML := makeRequest(http.MethodGet, "/services?delivery_mode=premises", nil, nil, "localhost")
	if modeResponse.StatusCode != http.StatusOK || !strings.Contains(modeHTML, `value="premises" selected`) || !strings.Contains(modeHTML, listing.Slug) {
		t.Fatal("HTML location filter not applied or retained")
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
	multilineDescription := "Consultation and haircut.\r\n\r\nBring your preferred style.\r\n<script>untrusted</script>"
	form := url.Values{"csrf_token": {session.CSRFToken}, "version": {"3"}, "title": {"<script>alert(1)</script>"}, "description": {multilineDescription}, "category": {"hair"}, "duration_minutes": {"30"}, "price_mode": {"fixed"}, "price": {"500.00"}, "currency": {"KES"}, "public_area": {"Westlands"}, "delivery_mode": {"premises"}, "state": {"published"}}
	response, pageHTML = makeRequest(http.MethodPost, editPath, form, seller, "partner.localhost")
	if response.StatusCode != http.StatusSeeOther {
		t.Fatalf("save status=%d body=%s", response.StatusCode, pageHTML)
	}
	response, pageHTML = makeRequest(http.MethodGet, "/services/"+listing.Slug, nil, nil, "localhost")
	if response.StatusCode != http.StatusOK || strings.Contains(pageHTML, "<script>alert(1)</script>") || !strings.Contains(pageHTML, "&lt;script&gt;") {
		t.Fatalf("unsafe or missing service page status=%d", response.StatusCode)
	}
	if !strings.Contains(pageHTML, `class="detail__desc"`) || !strings.Contains(pageHTML, "Consultation and haircut.\n\nBring your preferred style.\n&lt;script&gt;untrusted&lt;/script&gt;") {
		t.Fatal("multiline description was not preserved or escaped")
	}
	response, pageHTML = makeRequest(http.MethodPost, editPath, form, seller, "partner.localhost")
	if response.StatusCode != http.StatusConflict || !strings.Contains(pageHTML, "Your text is kept below") {
		t.Fatalf("stale form status=%d body=%s", response.StatusCode, pageHTML)
	}
	response, pageHTML = makeRequest(http.MethodGet, "/services?area=Westlands", nil, nil, "localhost")
	if response.StatusCode != http.StatusOK || !strings.Contains(pageHTML, listing.Slug) {
		t.Fatalf("directory status=%d body=%s", response.StatusCode, pageHTML)
	}
	// Server validation reports every failed field and retains malformed text.
	form.Set("duration_minutes", "3e1")
	form.Set("title", "x")
	form.Set("price", "1.005")
	response, pageHTML = makeRequest(http.MethodPost, editPath, form, seller, "partner.localhost")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid form status=%d body=%s", response.StatusCode, pageHTML)
	}
	for _, snippet := range []string{`<title>Error:`, `href="#duration_minutes"`, `aria-describedby="duration_minutes-error"`, `id="price-error"`, `value="3e1"`, `value="x"`} {
		if !strings.Contains(pageHTML, snippet) {
			t.Fatalf("validation HTML missing %q", snippet)
		}
	}
	bad := input
	bad.Title = "x"
	bad.DurationMinutes = 481
	bad.Price = "1.005"
	var invalid struct {
		Error struct {
			Code   string            `json:"code"`
			Fields map[string]string `json:"fields"`
		} `json:"error"`
	}
	if err := json.Unmarshal(request(http.MethodPatch, item, bad, "seller", true, http.StatusBadRequest), &invalid); err != nil {
		t.Fatal(err)
	}
	if invalid.Error.Code != "invalid_service" || len(invalid.Error.Fields) != 3 || invalid.Error.Fields["duration_minutes"] == "" {
		t.Fatalf("validation response=%+v", invalid)
	}
	latest, err := testStore.OwnService(t.Context(), int(mustUserID(t, "service-seller")), listing.ID)
	if err != nil {
		t.Fatal(err)
	}
	if latest.Version != 4 || latest.Title != "<script>alert(1)</script>" || latest.Description != strings.ReplaceAll(multilineDescription, "\r\n", "\n") {
		t.Fatal("invalid fields changed the stored listing")
	}
	response, _ = makeRequest(http.MethodGet, editPath, nil, other, "partner.localhost")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-shop HTML status=%d", response.StatusCode)
	}
}

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

	for _, state := range []string{"", "draft", "published", "archived"} {
		var ownPage struct {
			Items []db.ServiceListing `json:"items"`
			Total int                 `json:"total"`
		}
		decodeAPIData(t, request(http.MethodGet, collection+"?state="+state, nil, "seller", false, http.StatusOK), &ownPage)
		want := 0
		if state == "" || state == "draft" {
			want = 1
		}
		if ownPage.Total != want || len(ownPage.Items) != want {
			t.Fatalf("state=%q own page=%+v", state, ownPage)
		}
		decodeAPIData(t, request(http.MethodGet, collection+"?state="+state, nil, "other", false, http.StatusOK), &ownPage)
		if ownPage.Total != 0 || len(ownPage.Items) != 0 {
			t.Fatal("filtered API leaked another seller's draft")
		}
	}
	request(http.MethodGet, collection+"?state=unknown", nil, "seller", false, http.StatusBadRequest)
	for _, state := range []string{"draft", "published", "archived", "unknown"} {
		response, body := makeRequest(http.MethodGet, "/services?state="+state, nil, seller, "partner.localhost")
		wantStatus := http.StatusOK
		if state == "unknown" {
			wantStatus = http.StatusBadRequest
		}
		if response.StatusCode != wantStatus {
			t.Fatalf("state=%q status=%d body=%s", state, response.StatusCode, body)
		}
		if state != "unknown" && (!strings.Contains(body, `value="`+state+`" selected`) || strings.Contains(body, listing.Title) != (state == "draft")) {
			t.Fatalf("state=%q unexpected seller view: %s", state, body)
		}
	}
	pageResponse, _ := makeRequest(http.MethodGet, "/services?state=draft&page=2", nil, seller, "partner.localhost")
	if pageResponse.StatusCode != http.StatusSeeOther || pageResponse.Header.Get("Location") != "/services?state=draft" {
		t.Fatalf("filtered page redirect: status=%d location=%q", pageResponse.StatusCode, pageResponse.Header.Get("Location"))
	}
	item := fmt.Sprintf("%s/%d", collection, listing.ID)
	public := "/api/v1/catalog/services/" + listing.Slug
	request(http.MethodGet, public, nil, "anonymous", false, http.StatusNotFound)
	preview := fmt.Sprintf("/services/%d/preview", listing.ID)
	previewResponse, previewHTML := makeRequest(http.MethodGet, preview, nil, seller, "partner.localhost")
	if previewResponse.StatusCode != http.StatusOK || previewResponse.Header.Get("Cache-Control") != "no-store" || !strings.Contains(previewHTML, "Private preview of the saved listing · draft") || !strings.Contains(previewHTML, listing.Title) || strings.Contains(previewHTML, "Message service provider") {
		t.Fatalf("private preview status=%d body=%s", previewResponse.StatusCode, previewHTML)
	}
	crossResponse, _ := makeRequest(http.MethodGet, preview, nil, other, "partner.localhost")
	if crossResponse.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-shop preview status=%d", crossResponse.StatusCode)
	}
	for _, cookies := range [][]*http.Cookie{buyer, nil} {
		response, body := makeRequest(http.MethodGet, preview, nil, cookies, "partner.localhost")
		if response.StatusCode == http.StatusOK || strings.Contains(body, listing.Title) {
			t.Fatalf("private preview leaked: status=%d", response.StatusCode)
		}
	}
	previewResponse, _ = makeRequest(http.MethodHead, preview, nil, seller, "partner.localhost")
	if previewResponse.StatusCode != http.StatusOK {
		t.Fatalf("HEAD preview status=%d", previewResponse.StatusCode)
	}
	previewResponse, _ = makeRequest(http.MethodGet, "/services/invalid/preview", nil, seller, "partner.localhost")
	if previewResponse.StatusCode != http.StatusNotFound {
		t.Fatalf("invalid preview status=%d", previewResponse.StatusCode)
	}

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
	businessPath := fmt.Sprintf("/services?partner_id=%d&area=Westlands", listing.PartnerID)
	businessResponse, businessHTML := makeRequest(http.MethodGet, businessPath, nil, nil, "localhost")
	if businessResponse.StatusCode != http.StatusOK || !strings.Contains(businessHTML, listing.Slug) || !strings.Contains(businessHTML, fmt.Sprintf(`name="partner_id" value="%d"`, listing.PartnerID)) || !strings.Contains(detailHTML, fmt.Sprintf(`/services?partner_id=%d`, listing.PartnerID)) {
		t.Fatal("business discovery link or filter missing")
	}
	decodeAPIData(t, request(http.MethodGet, fmt.Sprintf("/api/v1/catalog/services?partner_id=%d", listing.PartnerID), nil, "anonymous", false, http.StatusOK), &page)
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].PartnerID != listing.PartnerID {
		t.Fatal("API business filter missing")
	}
	for _, raw := range []string{"0", "-1", "invalid", "2147483648", "999999999999999999999999"} {
		request(http.MethodGet, "/api/v1/catalog/services?partner_id="+raw, nil, "anonymous", false, http.StatusBadRequest)
		response, _ := makeRequest(http.MethodGet, "/services?partner_id="+raw, nil, nil, "localhost")
		if response.StatusCode != http.StatusBadRequest {
			t.Fatalf("partner_id=%s status=%d", raw, response.StatusCode)
		}
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
	previewResponse, previewHTML = makeRequest(http.MethodGet, preview, nil, seller, "partner.localhost")
	if previewResponse.StatusCode != http.StatusOK || !strings.Contains(previewHTML, "Private preview of the saved listing · archived") {
		t.Fatal("archived preview unavailable")
	}
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
	// A conflict can be resolved in place only after explicitly reviewing the
	// saved revision; another editor can still invalidate that revision.
	latest.Title = "Another editor's title"
	latest.Description = "Saved description with <script>saved</script>"
	latest, err = testStore.SaveService(t.Context(), int(mustUserID(t, "service-seller")), *latest)
	if err != nil {
		t.Fatal(err)
	}
	form.Set("version", "4")
	form.Set("duration_minutes", "45")
	form.Set("price", "600.25")
	form.Set("title", "My retained title")
	for attempt := 0; attempt < 2; attempt++ {
		response, pageHTML = makeRequest(http.MethodPost, editPath, form, seller, "partner.localhost")
		if response.StatusCode != http.StatusConflict {
			t.Fatalf("unreviewed retry status=%d body=%s", response.StatusCode, pageHTML)
		}
		for _, snippet := range []string{`value="My retained title"`, `Current saved version 5`, `name="reviewed_version" value="5" required`, `Saved description with &lt;script&gt;saved&lt;/script&gt;`} {
			if !strings.Contains(pageHTML, snippet) {
				t.Fatalf("conflict missing %q: %s", snippet, pageHTML)
			}
		}
		if strings.Contains(pageHTML, "<script>saved</script>") {
			t.Fatal("conflict snapshot was not escaped")
		}
	}
	form.Set("reviewed_version", "invalid")
	response, _ = makeRequest(http.MethodPost, editPath, form, seller, "partner.localhost")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("malformed acknowledgement status=%d", response.StatusCode)
	}
	latest.Title = "A newer intervening edit"
	_, err = testStore.SaveService(t.Context(), int(mustUserID(t, "service-seller")), *latest)
	if err != nil {
		t.Fatal(err)
	}
	form.Set("reviewed_version", "5")
	response, pageHTML = makeRequest(http.MethodPost, editPath, form, seller, "partner.localhost")
	if response.StatusCode != http.StatusConflict || !strings.Contains(pageHTML, `name="reviewed_version" value="6" required`) || !strings.Contains(pageHTML, `name="version" value="5"`) {
		t.Fatalf("intervening edit status=%d body=%s", response.StatusCode, pageHTML)
	}
	form.Set("version", "5")
	form.Set("reviewed_version", "6")
	response, pageHTML = makeRequest(http.MethodPost, editPath, form, seller, "partner.localhost")
	if response.StatusCode != http.StatusSeeOther {
		t.Fatalf("reviewed retry status=%d body=%s", response.StatusCode, pageHTML)
	}
	latest, err = testStore.OwnService(t.Context(), int(mustUserID(t, "service-seller")), listing.ID)
	if err != nil {
		t.Fatal(err)
	}
	if latest.Version != 7 || latest.Title != "My retained title" || latest.DurationMinutes != 45 || latest.PriceCents != 60025 {
		t.Fatalf("reviewed retry did not save retained values: %+v", latest)
	}
	otherSession, err := testStore.GetSession(t.Context(), other[0].Value)
	if err != nil {
		t.Fatal(err)
	}
	form.Set("csrf_token", otherSession.CSRFToken)
	response, pageHTML = makeRequest(http.MethodPost, editPath, form, other, "partner.localhost")
	if response.StatusCode != http.StatusNotFound || strings.Contains(pageHTML, "Current saved version") {
		t.Fatal("cross-shop conflict snapshot leak")
	}
	response, _ = makeRequest(http.MethodGet, editPath, nil, other, "partner.localhost")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-shop HTML status=%d", response.StatusCode)
	}
}

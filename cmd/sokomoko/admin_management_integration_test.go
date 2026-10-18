package main

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/kyambuthia/sokomoko/internal/db"
)

func TestIntegration_AdminReactivatesDeactivatedUser(t *testing.T) {
	clearAllTables()
	suffix := time.Now().UnixNano() % 1_000_000_000
	createTestUser(t, fmt.Sprintf("boss_%d", suffix), fmt.Sprintf("boss_%d@example.com", suffix), "AdminPass12345", "admin")
	worker := fmt.Sprintf("worker_%d", suffix)
	workerID := createTestUser(t, worker, worker+"@example.com", "StaffPass12345", "staff")
	admin := loginAndGetSessionCookie(t, "admin.localhost", fmt.Sprintf("boss_%d", suffix), "AdminPass12345")

	resp, _ := makeRequest(http.MethodPost, "/team", url.Values{"user_id": {fmt.Sprint(workerID)}}, []*http.Cookie{admin}, "admin.localhost")
	assertFlashRedirect(t, resp, "/team", "deactivated")
	if resp, _ := makeRequest(http.MethodPost, "/login", url.Values{"username": {worker}, "password": {"StaffPass12345"}}, nil, "admin.localhost"); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("deactivated login status=%d", resp.StatusCode)
	}
	if _, body := makeRequest(http.MethodGet, "/team", nil, []*http.Cookie{admin}, "admin.localhost"); !strings.Contains(body, "Deactivated") || !strings.Contains(body, `value="reactivate"`) {
		t.Fatal("team page does not list the deactivated user")
	}

	resp, _ = makeRequest(http.MethodPost, "/team", url.Values{"action": {"reactivate"}, "user_id": {fmt.Sprint(workerID)}}, []*http.Cookie{admin}, "admin.localhost")
	assertFlashRedirect(t, resp, "/team", "reactivated")
	loginAndGetSessionCookie(t, "admin.localhost", worker, "StaffPass12345")

	// Reactivating an active or unknown user reports it.
	resp, body := makeRequest(http.MethodPost, "/team", url.Values{"action": {"reactivate"}, "user_id": {fmt.Sprint(workerID)}}, []*http.Cookie{admin}, "admin.localhost")
	if resp.StatusCode == http.StatusFound || !strings.Contains(body, "User does not exist") {
		t.Fatalf("reactivate active user status=%d", resp.StatusCode)
	}
	var audits int
	if err := testStore.DB.QueryRow("SELECT count(*) FROM audit_logs WHERE action = 'user.reactivate'").Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("reactivate audits = %d, %v", audits, err)
	}
}

func TestIntegration_AdminRenamesCategoryAndPartner(t *testing.T) {
	clearAllTables()
	suffix := time.Now().UnixNano() % 1_000_000_000
	createTestUser(t, fmt.Sprintf("boss_%d", suffix), fmt.Sprintf("boss_%d@example.com", suffix), "AdminPass12345", "admin")
	admin := loginAndGetSessionCookie(t, "admin.localhost", fmt.Sprintf("boss_%d", suffix), "AdminPass12345")

	productID := createTestProduct(t, "Rename Lamp", fmt.Sprintf("rename-lamp-%d", suffix), 1000, 1)
	product, _ := testStore.GetProductByID(int(productID))
	categoryID := product.CategoryID.Int64
	category, _ := testStore.GetCategoryByID(int(categoryID))

	if _, body := makeRequest(http.MethodGet, fmt.Sprintf("/categories?edit=%d", categoryID), nil, []*http.Cookie{admin}, "admin.localhost"); !strings.Contains(body, "Edit category") || !strings.Contains(body, fmt.Sprintf(`name="category_id" value="%d"`, categoryID)) {
		t.Fatal("category edit form not shown")
	}
	resp, _ := makeRequest(http.MethodPost, "/categories", url.Values{"action": {"update"}, "category_id": {fmt.Sprint(categoryID)}, "name": {"Lighting"}, "description": {"Lamps and bulbs"}}, []*http.Cookie{admin}, "admin.localhost")
	assertFlashRedirect(t, resp, "/categories", "Category updated")
	renamed, _ := testStore.GetCategoryByID(int(categoryID))
	if renamed == nil || renamed.Name != "Lighting" || renamed.Description != "Lamps and bulbs" || renamed.Slug != category.Slug {
		t.Fatalf("category after rename = %+v", renamed)
	}
	resp, body := makeRequest(http.MethodPost, "/categories", url.Values{"action": {"update"}, "category_id": {fmt.Sprint(categoryID)}, "name": {""}}, []*http.Cookie{admin}, "admin.localhost")
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, "Enter a name") || !strings.Contains(body, "Edit category") {
		t.Fatalf("empty category name status=%d (form must stay in edit mode)", resp.StatusCode)
	}

	partnerID := createAPITestPartnerWithoutLogin(t, fmt.Sprintf("vendor-%d", suffix))
	if _, body := makeRequest(http.MethodGet, fmt.Sprintf("/partners?edit=%d", partnerID), nil, []*http.Cookie{admin}, "admin.localhost"); !strings.Contains(body, "Edit partner") {
		t.Fatal("partner edit form not shown")
	}
	resp, _ = makeRequest(http.MethodPost, "/partners", url.Values{"action": {"update"}, "partner_id": {fmt.Sprint(partnerID)}, "name": {"Vendor Renamed"}, "contact_email": {"Sales@Vendor.example"}}, []*http.Cookie{admin}, "admin.localhost")
	assertFlashRedirect(t, resp, "/partners", "Partner updated")
	partner, err := testStore.GetPartnerByID(int(partnerID))
	if err != nil || partner == nil || partner.Name != "Vendor Renamed" || partner.ContactEmail != "sales@vendor.example" || partner.Slug != fmt.Sprintf("vendor-%d", suffix) {
		t.Fatalf("partner after rename = %+v, %v", partner, err)
	}
	resp, body = makeRequest(http.MethodPost, "/partners", url.Values{"action": {"update"}, "partner_id": {fmt.Sprint(partnerID)}, "name": {"Bad"}, "contact_email": {"not-an-email"}}, []*http.Cookie{admin}, "admin.localhost")
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, "contact email must be valid") {
		t.Fatalf("bad partner email status=%d", resp.StatusCode)
	}
}

func createAPITestPartnerWithoutLogin(t *testing.T, slug string) int64 {
	t.Helper()
	id, err := testStore.UpsertPartner(db.Partner{Name: slug, Slug: slug, ContactEmail: slug + "@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

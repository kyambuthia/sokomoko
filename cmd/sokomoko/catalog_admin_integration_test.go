package main

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestIntegration_AdminCategories(t *testing.T) {
	clearAllTables()
	suffix := time.Now().UnixNano() % 1_000_000_000
	createTestUser(t, fmt.Sprintf("cat_admin_%d", suffix), fmt.Sprintf("cat_admin_%d@example.com", suffix), "adminpass123", "admin")
	staff := fmt.Sprintf("cat_staff_%d", suffix)
	createTestUser(t, staff, staff+"@example.com", "staffpass123", "staff")
	cookie := loginAndGetSessionCookie(t, "admin.localhost", staff, "staffpass123")

	resp, _ := makeRequest(http.MethodPost, "/categories", url.Values{"name": {"Garden Tools"}, "description": {"Spades and rakes"}}, []*http.Cookie{cookie}, "admin.localhost")
	assertFlashRedirect(t, resp, "/categories", "Category created")
	category, err := testStore.GetCategoryBySlug("garden-tools")
	if err != nil || category == nil || category.Name != "Garden Tools" {
		t.Fatalf("category not created with derived slug: %+v %v", category, err)
	}

	resp, body := makeRequest(http.MethodPost, "/categories", url.Values{"name": {"garden tools"}}, []*http.Cookie{cookie}, "admin.localhost")
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, "already exists") {
		t.Fatalf("duplicate category status=%d", resp.StatusCode)
	}
	resp, _ = makeRequest(http.MethodPost, "/categories", url.Values{"name": {"Bad"}, "slug": {"Bad Slug"}}, []*http.Cookie{cookie}, "admin.localhost")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid slug status=%d", resp.StatusCode)
	}

	resp, body = makeRequest(http.MethodGet, "/categories", nil, []*http.Cookie{cookie}, "admin.localhost")
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "Garden Tools") {
		t.Fatalf("category list status=%d", resp.StatusCode)
	}

	resp, _ = makeRequest(http.MethodPost, "/categories", url.Values{"action": {"delete"}, "category_id": {fmt.Sprint(category.ID)}}, []*http.Cookie{cookie}, "admin.localhost")
	assertFlashRedirect(t, resp, "/categories", "Category deleted")
	if category, _ := testStore.GetCategoryBySlug("garden-tools"); category != nil {
		t.Fatal("category was not deleted")
	}

	customer := fmt.Sprintf("cat_customer_%d", suffix)
	createTestUser(t, customer, customer+"@example.com", "custpass1234", "user")
	resp, _ = makeRequest(http.MethodGet, "/categories", nil, nil, "admin.localhost")
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("anonymous /categories status=%d, want redirect to login", resp.StatusCode)
	}
}

func TestIntegration_AdminPartnersOnboardingAndSuspension(t *testing.T) {
	clearAllTables()
	suffix := time.Now().UnixNano() % 1_000_000_000
	admin := fmt.Sprintf("p_admin_%d", suffix)
	staff := fmt.Sprintf("p_staff_%d", suffix)
	createTestUser(t, admin, admin+"@example.com", "adminpass123", "admin")
	createTestUser(t, staff, staff+"@example.com", "staffpass123", "staff")
	if _, err := testStore.DB.Exec(`INSERT INTO store_settings (id, store_name, store_slug, contact_email) VALUES (1, 'Store', 'store', 'store@example.com')`); err != nil {
		t.Fatal(err)
	}

	staffCookie := loginAndGetSessionCookie(t, "admin.localhost", staff, "staffpass123")
	if resp, _ := makeRequest(http.MethodGet, "/partners", nil, []*http.Cookie{staffCookie}, "admin.localhost"); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("staff /partners status=%d, want 403", resp.StatusCode)
	}

	adminCookie := loginAndGetSessionCookie(t, "admin.localhost", admin, "adminpass123")
	resp, _ := makeRequest(http.MethodPost, "/partners", url.Values{"name": {"Maji Crafts"}, "contact_email": {"hello@maji.example"}}, []*http.Cookie{adminCookie}, "admin.localhost")
	assertFlashRedirect(t, resp, "/partners", "Partner created")
	partner, err := testStore.GetPartnerBySlug("maji-crafts")
	if err != nil || partner == nil || partner.Status != "active" {
		t.Fatalf("partner not created: %+v %v", partner, err)
	}
	if resp, _ := makeRequest(http.MethodPost, "/partners", url.Values{"name": {"Maji Crafts"}}, []*http.Cookie{adminCookie}, "admin.localhost"); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("duplicate partner status=%d", resp.StatusCode)
	}

	login := fmt.Sprintf("maji_%d", suffix)
	resp, body := makeRequest(http.MethodPost, "/partners", url.Values{
		"action": {"create_account"}, "partner_id": {fmt.Sprint(partner.ID)},
		"username": {login}, "email": {login + "@example.com"}, "password": {"short"},
	}, []*http.Cookie{adminCookie}, "admin.localhost")
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, "Password must be 10 to 72 characters") {
		t.Fatalf("weak password status=%d", resp.StatusCode)
	}
	resp, _ = makeRequest(http.MethodPost, "/partners", url.Values{
		"action": {"create_account"}, "partner_id": {fmt.Sprint(partner.ID)},
		"username": {login}, "email": {login + "@example.com"}, "password": {"partnerpass123"},
	}, []*http.Cookie{adminCookie}, "admin.localhost")
	assertFlashRedirect(t, resp, "/partners", "Partner login created")

	productID := createTestProduct(t, "Maji Basket", fmt.Sprintf("maji-basket-%d", suffix), 2500, 4)
	if _, err := testStore.DB.Exec("UPDATE products SET partner_id = $1 WHERE id = $2", partner.ID, productID); err != nil {
		t.Fatal(err)
	}
	slugPath := fmt.Sprintf("/products/maji-basket-%d", suffix)

	partnerCookie := loginAndGetSessionCookie(t, "partner.localhost", login, "partnerpass123")
	resp, body = makeRequest(http.MethodGet, "/products", nil, []*http.Cookie{partnerCookie}, "partner.localhost")
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "Maji Basket") {
		t.Fatalf("partner login cannot see its product: status=%d", resp.StatusCode)
	}
	if resp, _ := makeRequest(http.MethodGet, slugPath, nil, nil, "localhost"); resp.StatusCode != http.StatusOK {
		t.Fatalf("active partner product status=%d", resp.StatusCode)
	}

	resp, _ = makeRequest(http.MethodPost, "/partners", url.Values{"action": {"status"}, "status": {"suspended"}, "partner_id": {fmt.Sprint(partner.ID)}}, []*http.Cookie{adminCookie}, "admin.localhost")
	assertFlashRedirect(t, resp, "/partners", "Partner suspended")
	if resp, _ := makeRequest(http.MethodGet, slugPath, nil, nil, "localhost"); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("suspended partner product status=%d, want 404", resp.StatusCode)
	}
	if _, body := makeRequest(http.MethodGet, "/", nil, nil, "localhost"); strings.Contains(body, "Maji Basket") {
		t.Fatal("suspended partner product listed on the storefront")
	}
	if _, body := makeRequest(http.MethodGet, "/products", nil, []*http.Cookie{partnerCookie}, "partner.localhost"); strings.Contains(body, "Maji Basket") {
		t.Fatal("suspended partner login still sees partner products")
	}

	resp, _ = makeRequest(http.MethodPost, "/partners", url.Values{"action": {"status"}, "status": {"active"}, "partner_id": {fmt.Sprint(partner.ID)}}, []*http.Cookie{adminCookie}, "admin.localhost")
	assertFlashRedirect(t, resp, "/partners", "Partner reactivated")
	if resp, _ := makeRequest(http.MethodGet, slugPath, nil, nil, "localhost"); resp.StatusCode != http.StatusOK {
		t.Fatalf("reactivated partner product status=%d", resp.StatusCode)
	}
}

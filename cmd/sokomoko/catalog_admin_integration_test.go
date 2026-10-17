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

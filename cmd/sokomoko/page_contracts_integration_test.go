package main

import (
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/kyambuthia/sokomoko/internal/db"
)

// Exercise the redesigned templates with PostgreSQL view models, including
// nonempty lists that would otherwise hide missing fields and money formatting.
func TestIntegration_PageContracts(t *testing.T) {
	clearAllTables()
	createTestUser(t, "page_admin", "page_admin@example.com", "strongpass123", "admin")
	createTestUser(t, "page_buyer", "page_buyer@example.com", "strongpass123", "user")
	productID := createTestProduct(t, "Page Contract Lamp", "page-contract-lamp", 2450, 10)
	if err := testStore.UpsertStoreSettings(db.StoreSettings{StoreName: "Page Store", StoreSlug: "page-store", ContactEmail: "store@example.com"}); err != nil {
		t.Fatal(err)
	}
	userID := int(mustUserID(t, "page_buyer"))
	if err := testStore.AddToCart(t.Context(), userID, int(productID), 1); err != nil {
		t.Fatal(err)
	}
	if _, err := testStore.PlaceOrderFromCart(userID, "Page Lane"); err != nil {
		t.Fatal(err)
	}
	if err := testStore.AddToCart(t.Context(), userID, int(productID), 1); err != nil {
		t.Fatal(err)
	}
	buyer := loginAndGetSessionCookie(t, "", "page_buyer", "strongpass123")
	admin := loginAndGetSessionCookie(t, "admin.localhost", "page_admin", "strongpass123")
	for _, tc := range []struct {
		host, path, content string
		cookie              *http.Cookie
	}{
		{"", "/", "24.50", nil},
		{"", "/products/page-contract-lamp", "24.50", nil},
		{"", "/search?q=Lamp", "24.50", nil},
		{"", "/cart", "24.50", buyer},
		{"", "/checkout", "24.50", buyer},
		{"", "/account", "24.50", buyer},
		{"admin.localhost", "/", "Admin Dashboard", admin},
		{"admin.localhost", "/products", "24.50", admin},
		{"admin.localhost", "/orders", "Page Contract Lamp", admin},
		{"admin.localhost", "/reports", "Reports", admin},
		{"admin.localhost", "/team", "page_admin", admin},
		{"partner.localhost", "/dashboard", "Page Store", admin},
		{"partner.localhost", "/products", "24.50", admin},
		{"partner.localhost", "/products/new", "csrf_token", admin},
		{"partner.localhost", "/orders", "Page Contract Lamp", admin},
	} {
		t.Run(tc.host+tc.path, func(t *testing.T) {
			var cookies []*http.Cookie
			if tc.cookie != nil {
				cookies = append(cookies, tc.cookie)
			}
			resp, body := makeRequest(http.MethodGet, tc.path, nil, cookies, tc.host)
			if tc.cookie != nil {
				for _, form := range regexp.MustCompile(`(?s)<form\b[^>]*method="POST"[^>]*>.*?</form>`).FindAllString(body, -1) {
					if !strings.Contains(form, `name="csrf_token"`) {
						t.Errorf("authenticated form lacks CSRF field: %s", form)
					}
				}
			}
			if resp.StatusCode != http.StatusOK || !strings.Contains(body, tc.content) || strings.Contains(body, "%!") {
				t.Fatalf("status=%d; expected rendered content %q without formatting errors", resp.StatusCode, tc.content)
			}
		})
	}
}

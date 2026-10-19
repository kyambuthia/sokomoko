package main

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestIntegration_TeamPageSearchesAndPages(t *testing.T) {
	clearAllTables()
	createTestUser(t, "team_boss", "team_boss@example.com", "AdminPass12345", "admin")
	if _, err := testStore.DB.Exec(`INSERT INTO users (username, email, password_hash, role)
		SELECT 'shopper_' || lpad(n::text, 2, '0'), 'shopper' || n || '@example.com', 'x', 'user'
		FROM generate_series(0, 59) AS n`); err != nil {
		t.Fatal(err)
	}
	admin := loginAndGetSessionCookie(t, "admin.localhost", "team_boss", "AdminPass12345")
	get := func(path string) (*http.Response, string) {
		return makeRequest(http.MethodGet, path, nil, []*http.Cookie{admin}, "admin.localhost")
	}

	// 61 accounts at 50 a page: the admin sorts first, so the last shoppers
	// land on page 2.
	resp, body := get("/team")
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "Page 1 of 2") || !strings.Contains(body, `href="/team?page=2"`) ||
		!strings.Contains(body, "shopper_48") || strings.Contains(body, "shopper_49") {
		t.Fatalf("page 1 status=%d", resp.StatusCode)
	}
	if _, body := get("/team?page=2"); !strings.Contains(body, "shopper_59") || strings.Contains(body, "shopper_00") || !strings.Contains(body, `href="/team"`) {
		t.Fatal("page 2 does not continue the list")
	}
	if resp, _ := get("/team?page=7"); resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/team?page=2" {
		t.Fatalf("past the end status=%d location=%q", resp.StatusCode, resp.Header.Get("Location"))
	}

	// Search covers every page and keeps the query in the field.
	if _, body := get("/team?q=shopper_59"); !strings.Contains(body, "shopper_59") || strings.Contains(body, "shopper_58") ||
		strings.Contains(body, "Page 1 of") || !strings.Contains(body, `value="shopper_59"`) {
		t.Fatal("search did not find the account on page 2")
	}
	if _, body := get("/team?q=nobody"); !strings.Contains(body, "No one matches") {
		t.Fatal("empty search has no message")
	}
}

func TestIntegration_AccountPageListsOrdersInPages(t *testing.T) {
	clearAllTables()
	buyerID := createTestUser(t, "pager_buyer", "pager_buyer@example.com", "BuyerPass12345", "user")
	productID := createTestProduct(t, "Pager Mug", "pager-mug", 500, 100)
	for i := 0; i < accountPageOrders+1; i++ {
		if err := testStore.AddToCart(t.Context(), int(buyerID), int(productID), 1); err != nil {
			t.Fatal(err)
		}
		if _, err := testStore.PlaceOrderFromCart(t.Context(), int(buyerID), "1 Pager Road"); err != nil {
			t.Fatal(err)
		}
	}
	buyer := loginAndGetSessionCookie(t, "", "pager_buyer", "BuyerPass12345")

	var oldest int
	if err := testStore.DB.QueryRow("SELECT min(id) FROM orders WHERE user_id = $1", buyerID).Scan(&oldest); err != nil {
		t.Fatal(err)
	}
	oldestCard := fmt.Sprintf(`id="order-%d"`, oldest)
	_, body := makeRequest(http.MethodGet, "/account", nil, []*http.Cookie{buyer}, "")
	if strings.Count(body, "data-order>") != accountPageOrders || strings.Contains(body, oldestCard) || !strings.Contains(body, `href="/account?page=2"`) {
		t.Fatalf("page 1 shows %d orders", strings.Count(body, "data-order>"))
	}
	_, body = makeRequest(http.MethodGet, "/account?page=2", nil, []*http.Cookie{buyer}, "")
	if strings.Count(body, "data-order>") != 1 || !strings.Contains(body, oldestCard) {
		t.Fatalf("page 2 shows %d orders", strings.Count(body, "data-order>"))
	}
}

// accountPageOrders mirrors routes.accountOrdersPerPage.
const accountPageOrders = 20

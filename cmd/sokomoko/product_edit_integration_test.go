package main

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestIntegration_PartnerEditsOwnProductOnly(t *testing.T) {
	clearAllTables()
	suffix := time.Now().UnixNano() % 1_000_000_000
	setupPartnerStore(t, suffix)

	owner := fmt.Sprintf("owner_%d", suffix)
	other := fmt.Sprintf("other_%d", suffix)
	createTestUser(t, owner, owner+"@example.com", "PartnerPass123", "partner")
	createTestUser(t, other, other+"@example.com", "PartnerPass123", "partner")
	ownerPartner := createAPITestPartner(t, owner)
	otherPartner := createAPITestPartner(t, other)
	createTestUser(t, fmt.Sprintf("root_%d", suffix), fmt.Sprintf("root_%d@example.com", suffix), "AdminPass12345", "admin")

	mine := createTestProduct(t, "Kikapu Basket", fmt.Sprintf("kikapu-%d", suffix), 2500, 10)
	theirs := createTestProduct(t, "Other Stool", fmt.Sprintf("stool-%d", suffix), 4000, 3)
	if _, err := testStore.DB.Exec("UPDATE products SET partner_id = $1 WHERE id = $2", ownerPartner, mine); err != nil {
		t.Fatal(err)
	}
	if _, err := testStore.DB.Exec("UPDATE products SET partner_id = $1 WHERE id = $2", otherPartner, theirs); err != nil {
		t.Fatal(err)
	}
	// A customer holds two units in checkout; editing stock must not touch them.
	buyer := createTestUser(t, fmt.Sprintf("buyer_%d", suffix), fmt.Sprintf("buyer_%d@example.com", suffix), "BuyerPass12345", "user")
	if err := testStore.AddToCart(t.Context(), int(buyer), int(mine), 2); err != nil {
		t.Fatal(err)
	}
	if err := testStore.ReserveCartForCheckout(t.Context(), int(buyer), "edit-test", time.Now().Add(15*time.Minute)); err != nil {
		t.Fatal(err)
	}

	cookie := loginAndGetSessionCookie(t, "partner.localhost", owner, "PartnerPass123")
	editPath := fmt.Sprintf("/products/%d/edit", mine)

	resp, body := makeRequest(http.MethodGet, "/products", nil, []*http.Cookie{cookie}, "partner.localhost")
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, editPath) {
		t.Fatalf("product list has no edit link: status=%d", resp.StatusCode)
	}
	resp, body = makeRequest(http.MethodGet, editPath, nil, []*http.Cookie{cookie}, "partner.localhost")
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `value="Kikapu Basket"`) || !strings.Contains(body, `value="25.00"`) || !strings.Contains(body, `value="8"`) {
		t.Fatalf("edit form status=%d does not show current values (available stock 8)", resp.StatusCode)
	}

	// Invalid input is re-shown with the error.
	resp, body = makeRequest(http.MethodPost, editPath, url.Values{"name": {"Renamed"}, "price": {"abc"}, "stock_quantity": {"5"}}, []*http.Cookie{cookie}, "partner.localhost")
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, "Price must be an amount") || !strings.Contains(body, `value="Renamed"`) {
		t.Fatalf("invalid edit status=%d", resp.StatusCode)
	}

	resp, _ = makeRequest(http.MethodPost, editPath, url.Values{
		"name": {"Kikapu Basket Large"}, "description": {"Hand woven"}, "price": {"31.50"}, "stock_quantity": {"5"},
	}, []*http.Cookie{cookie}, "partner.localhost")
	assertFlashRedirect(t, resp, "/products", "Product updated")
	product, err := testStore.GetProductByID(int(mine))
	if err != nil || product == nil {
		t.Fatal(err)
	}
	if product.Name != "Kikapu Basket Large" || product.Price != 3150 || product.StockQuantity != 5 || product.Description != "Hand woven" {
		t.Fatalf("product after edit = %+v", product)
	}
	if product.Slug != fmt.Sprintf("kikapu-%d", suffix) {
		t.Fatalf("slug changed to %s", product.Slug)
	}
	var reserved, onHand int
	if err := testStore.DB.QueryRow("SELECT reserved_quantity, on_hand_quantity FROM inventory_stocks WHERE product_id = $1", mine).Scan(&reserved, &onHand); err != nil || reserved != 2 || onHand != 7 {
		t.Fatalf("inventory reserved=%d on_hand=%d err=%v", reserved, onHand, err)
	}
	var audits int
	if err := testStore.DB.QueryRow("SELECT count(*) FROM audit_logs WHERE action = 'product.update' AND target_id = $1", mine).Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("audit rows = %d, %v", audits, err)
	}

	// Another partner's product reads as missing, for viewing and saving.
	otherPath := fmt.Sprintf("/products/%d/edit", theirs)
	if resp, _ := makeRequest(http.MethodGet, otherPath, nil, []*http.Cookie{cookie}, "partner.localhost"); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-partner edit form status=%d", resp.StatusCode)
	}
	resp, _ = makeRequest(http.MethodPost, otherPath, url.Values{"name": {"Stolen"}, "price": {"1.00"}, "stock_quantity": {"0"}}, []*http.Cookie{cookie}, "partner.localhost")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-partner save status=%d", resp.StatusCode)
	}
	if p, _ := testStore.GetProductByID(int(theirs)); p == nil || p.Name != "Other Stool" {
		t.Fatalf("other partner's product changed: %+v", p)
	}
	if resp, _ := makeRequest(http.MethodGet, "/products/999999/edit", nil, []*http.Cookie{cookie}, "partner.localhost"); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing product status=%d", resp.StatusCode)
	}

	// Admins can edit any product.
	admin := loginAndGetSessionCookie(t, "partner.localhost", fmt.Sprintf("root_%d", suffix), "AdminPass12345")
	resp, _ = makeRequest(http.MethodPost, otherPath, url.Values{"name": {"Other Stool"}, "price": {"45.00"}, "stock_quantity": {"3"}}, []*http.Cookie{admin}, "partner.localhost")
	assertFlashRedirect(t, resp, "/products", "Product updated")
	if p, _ := testStore.GetProductByID(int(theirs)); p == nil || p.Price != 4500 {
		t.Fatalf("admin edit not saved: %+v", p)
	}

	// The JSON API changes only the fields it is sent.
	apiPath := fmt.Sprintf("/api/v1/partner/products/%d", mine)
	resp, raw := makeAPIRequestOnHost(t, "partner.localhost", http.MethodPatch, apiPath, map[string]string{"price": "29.00"}, []*http.Cookie{cookie}, true, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("API patch status=%d body=%s", resp.StatusCode, raw)
	}
	if p, _ := testStore.GetProductByID(int(mine)); p == nil || p.Price != 2900 || p.Name != "Kikapu Basket Large" || p.StockQuantity != 5 {
		t.Fatalf("API patch result = %+v", p)
	}
	resp, _ = makeAPIRequestOnHost(t, "partner.localhost", http.MethodPatch, fmt.Sprintf("/api/v1/partner/products/%d", theirs), map[string]string{"price": "1.00"}, []*http.Cookie{cookie}, true, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("API cross-partner patch status=%d", resp.StatusCode)
	}
	resp, _ = makeAPIRequestOnHost(t, "partner.localhost", http.MethodPatch, apiPath, map[string]string{"stock": "-1"}, []*http.Cookie{cookie}, true, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("API invalid patch status=%d", resp.StatusCode)
	}
}

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type apiEnvelope struct {
	Data  json.RawMessage `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func makeAPIRequest(t *testing.T, method, path string, body any, cookies []*http.Cookie, csrf bool, headers map[string]string) (*http.Response, []byte) {
	return makeAPIRequestOnHost(t, "localhost", method, path, body, cookies, csrf, headers)
}

func makeAPIRequestOnHost(t *testing.T, host, method, path string, body any, cookies []*http.Cookie, csrf bool, headers map[string]string) (*http.Response, []byte) {
	t.Helper()
	var bodyReader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal API body: %v", err)
		}
		bodyReader = bytes.NewReader(encoded)
	}
	req := httptest.NewRequest(method, "http://"+host+path, bodyReader)
	req.Host = host
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	if csrf && (method == http.MethodPost || method == http.MethodPatch || method == http.MethodDelete) {
		for _, cookie := range cookies {
			if cookie == nil || cookie.Name != "session_token" {
				continue
			}
			session, err := testStore.GetSession(cookie.Value)
			if err == nil && session != nil {
				req.Header.Set("X-CSRF-Token", session.CSRFToken)
			}
		}
	}
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	recorder := httptest.NewRecorder()
	testHandler.ServeHTTP(recorder, req)
	resp := recorder.Result()
	data, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("read API response: %v", err)
	}
	return resp, data
}

func decodeAPIData(t *testing.T, body []byte, target any) {
	t.Helper()
	var envelope apiEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode API envelope: %v; body=%s", err, body)
	}
	if envelope.Error != nil {
		t.Fatalf("API returned error %s: %s", envelope.Error.Code, envelope.Error.Message)
	}
	if err := json.Unmarshal(envelope.Data, target); err != nil {
		t.Fatalf("decode API data: %v; body=%s", err, body)
	}
}

func TestAPI_CatalogSearchUsesJSONContract(t *testing.T) {
	clearAllTables()
	productID := createTestProduct(t, "API Lamp", fmt.Sprintf("api-lamp-%d", time.Now().UnixNano()), 24.5, 3)
	if productID == 0 {
		t.Fatal("expected product id")
	}

	resp, body := makeAPIRequest(t, http.MethodGet, "/api/v1/catalog/search?q=API+Lamp&limit=6", nil, nil, false, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("catalog search status=%d body=%s", resp.StatusCode, body)
	}
	var data struct {
		Items []struct {
			ID            int    `json:"id"`
			Name          string `json:"name"`
			StockQuantity int    `json:"stock_quantity"`
		} `json:"items"`
	}
	decodeAPIData(t, body, &data)
	if len(data.Items) != 1 || data.Items[0].ID != int(productID) || data.Items[0].Name != "API Lamp" || data.Items[0].StockQuantity != 3 {
		t.Fatalf("unexpected catalog response: %#v", data.Items)
	}
}

func TestAPI_ProtectedMutationRequiresJSONCSRF(t *testing.T) {
	clearAllTables()
	suffix := time.Now().UnixNano()
	username := fmt.Sprintf("api_cart_%d", suffix)
	password := "strongpass123"
	createTestUser(t, username, fmt.Sprintf("%s@example.com", username), password, "user")
	cookie := loginAndGetSessionCookie(t, "", username, password)
	productID := createTestProduct(t, "API Cart Product", fmt.Sprintf("api-cart-%d", suffix), 9, 5)

	resp, body := makeAPIRequest(t, http.MethodPost, "/api/v1/cart/items", map[string]any{"product_id": productID, "quantity": 1}, []*http.Cookie{cookie}, false, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("missing CSRF status=%d body=%s", resp.StatusCode, body)
	}
	var envelope apiEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil || envelope.Error == nil || envelope.Error.Code != "csrf_token_missing" {
		t.Fatalf("expected JSON CSRF error, body=%s", body)
	}

	resp, body = makeAPIRequest(t, http.MethodPost, "/api/v1/cart/items", map[string]any{"product_id": productID, "quantity": 1}, []*http.Cookie{cookie}, true, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("cart add status=%d body=%s", resp.StatusCode, body)
	}
	var cart struct {
		Items []struct {
			ProductID int `json:"product_id"`
			Quantity  int `json:"quantity"`
		} `json:"items"`
	}
	decodeAPIData(t, body, &cart)
	if len(cart.Items) != 1 || cart.Items[0].ProductID != int(productID) || cart.Items[0].Quantity != 1 {
		t.Fatalf("unexpected cart: %#v", cart.Items)
	}
}

func TestAPI_CheckoutPlacementIsIdempotent(t *testing.T) {
	clearAllTables()
	suffix := time.Now().UnixNano()
	username := fmt.Sprintf("api_checkout_%d", suffix)
	password := "strongpass123"
	createTestUser(t, username, fmt.Sprintf("%s@example.com", username), password, "user")
	cookie := loginAndGetSessionCookie(t, "", username, password)
	productID := createTestProduct(t, "API Checkout Product", fmt.Sprintf("api-checkout-%d", suffix), 10, 5)

	resp, body := makeAPIRequest(t, http.MethodPost, "/api/v1/cart/items", map[string]any{"product_id": productID, "quantity": 2}, []*http.Cookie{cookie}, true, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("cart add status=%d body=%s", resp.StatusCode, body)
	}
	resp, body = makeAPIRequest(t, http.MethodPost, "/api/v1/checkout", map[string]any{}, []*http.Cookie{cookie}, true, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("checkout prepare status=%d body=%s", resp.StatusCode, body)
	}
	var checkout struct {
		Token string `json:"token"`
	}
	decodeAPIData(t, body, &checkout)
	if strings.TrimSpace(checkout.Token) == "" {
		t.Fatal("expected checkout token")
	}

	path := "/api/v1/checkout/" + checkout.Token + "/place-order"
	requestHeaders := map[string]string{"Idempotency-Key": checkout.Token}
	requestBody := map[string]any{"delivery_address": "API Checkout Lane", "payment_method": "cash_on_delivery"}
	resp, body = makeAPIRequest(t, http.MethodPost, path, requestBody, []*http.Cookie{cookie}, true, requestHeaders)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("checkout placement status=%d body=%s", resp.StatusCode, body)
	}
	resp, body = makeAPIRequest(t, http.MethodPost, path, requestBody, []*http.Cookie{cookie}, true, requestHeaders)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("checkout replay status=%d body=%s", resp.StatusCode, body)
	}

	orders, err := testStore.ListOrdersByUser(int(mustUserID(t, username)))
	if err != nil {
		t.Fatalf("list orders: %v", err)
	}
	if len(orders) != 1 {
		t.Fatalf("expected one order after replay, got %d", len(orders))
	}
}

func TestAPI_PartnerWorkspaceIsScopedToPartnerIdentity(t *testing.T) {
	clearAllTables()
	if _, err := testStore.DB.Exec(`INSERT INTO store_settings (id, store_name, store_slug, description, contact_email) VALUES (1, 'API Store', 'api-store', 'API test store', 'store@example.com')`); err != nil {
		t.Fatalf("create store settings: %v", err)
	}

	suffix := time.Now().UnixNano()
	partnerA := fmt.Sprintf("partner_a_%d", suffix)
	partnerB := fmt.Sprintf("partner_b_%d", suffix)
	password := "strongpass123"
	createTestUser(t, partnerA, partnerA+"@example.com", password, "partner")
	createTestUser(t, partnerB, partnerB+"@example.com", password, "partner")
	partnerAID := mustUserID(t, partnerA)
	partnerBID := mustUserID(t, partnerB)

	productA := createTestProduct(t, "Partner A Product", fmt.Sprintf("partner-a-%d", suffix), 10, 5)
	productB := createTestProduct(t, "Partner B Product", fmt.Sprintf("partner-b-%d", suffix), 12, 5)
	if _, err := testStore.DB.Exec("UPDATE products SET partner_id = ? WHERE id IN (?, ?)", partnerAID, productA, productB); err != nil {
		t.Fatalf("assign products: %v", err)
	}
	if _, err := testStore.DB.Exec("UPDATE products SET partner_id = ? WHERE id = ?", partnerBID, productB); err != nil {
		t.Fatalf("assign partner B product: %v", err)
	}

	customer := fmt.Sprintf("partner_customer_%d", suffix)
	createTestUser(t, customer, customer+"@example.com", password, "user")
	customerID := mustUserID(t, customer)
	if err := testStore.AddToCart(int(customerID), int(productB), 1); err != nil {
		t.Fatalf("add customer cart: %v", err)
	}
	orderID, err := testStore.PlaceOrderFromCart(int(customerID), "Partner API Lane")
	if err != nil {
		t.Fatalf("create partner-scoping order: %v", err)
	}

	cookie := loginAndGetSessionCookie(t, "partner.localhost", partnerA, password)
	resp, body := makeAPIRequestOnHost(t, "partner.localhost", http.MethodGet, "/api/v1/partner/products", nil, []*http.Cookie{cookie}, false, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("partner products status=%d body=%s", resp.StatusCode, body)
	}
	var products struct {
		Items []struct {
			Name string `json:"name"`
		} `json:"items"`
	}
	decodeAPIData(t, body, &products)
	if len(products.Items) != 1 || products.Items[0].Name != "Partner A Product" {
		t.Fatalf("partner A saw unexpected products: %#v", products.Items)
	}

	resp, body = makeAPIRequestOnHost(t, "partner.localhost", http.MethodPatch, fmt.Sprintf("/api/v1/partner/orders/%d", orderID), map[string]any{
		"partner_status": "accepted", "delivery_status": "processing", "delivery_notice": "not yours",
	}, []*http.Cookie{cookie}, true, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-partner update status=%d body=%s", resp.StatusCode, body)
	}
}

func mustUserID(t *testing.T, username string) int64 {
	t.Helper()
	user, err := testStore.GetUserByUsername(username)
	if err != nil || user == nil {
		t.Fatalf("find user: %v", err)
	}
	return int64(user.ID)
}

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

	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/money"
)

func apiJSON(t *testing.T, method, path, host string, body any, headers map[string]string, cookies []*http.Cookie) (*http.Response, map[string]any) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal %s %s: %v", method, path, err)
		}
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, reader)
	if host != "" {
		req.Host = host
	} else {
		req.Host = "localhost"
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	testHandler.ServeHTTP(rec, req)
	resp := rec.Result()
	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s %s: %v", method, path, err)
	}
	var parsed map[string]any
	if len(bytes.TrimSpace(rawBody)) > 0 && strings.Contains(resp.Header.Get("Content-Type"), "application/json") {
		if err := json.Unmarshal(rawBody, &parsed); err != nil {
			t.Fatalf("unmarshal %s %s (%d): %v\n%s", method, path, resp.StatusCode, err, rawBody)
		}
	}
	return resp, parsed
}

func TestIntegration_AgentJourney(t *testing.T) {
	// Seed an active partner store and an in-stock product attached to it.
	partnerID, err := testStore.CreatePartner(t.Context(), db.Partner{Name: "Agent Goods", Slug: "agent-goods", ContactEmail: "agent-goods@example.com"})
	if err != nil {
		t.Fatalf("CreatePartner: %v", err)
	}
	catID, err := testStore.CreateCategory(t.Context(), db.Category{
		Name:        "Agent Gear",
		Slug:        "agent-gear",
		Description: "Gear for agents",
	})
	if err != nil {
		t.Fatalf("CreateCategory: %v", err)
	}
	productID, err := testStore.CreateProduct(t.Context(), db.Product{
		Name:          "Field Compass",
		Slug:          "field-compass",
		Description:   "Brass compass",
		Price:         money.Cents(2500),
		StockQuantity: 10,
		CategoryID:    sqlNullInt64(catID),
		PartnerID:     sqlNullInt64(partnerID),
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}

	// 1. Discovery surfaces: /robots.txt, /llms.txt, /sitemap.xml, HTML <link>, /api/v1/openapi.json.
	for _, tc := range []struct {
		path string
		want string
	}{
		{"/robots.txt", "Allow: /api/v1/openapi.json"},
		{"/llms.txt", "/api/v1/openapi.json"},
		{"/sitemap.xml", "/products/field-compass"},
		{"/", `href="/api/v1/openapi.json"`},
	} {
		resp, raw := makeRequest(http.MethodGet, tc.path, nil, nil, "")
		if resp.StatusCode != http.StatusOK || !strings.Contains(raw, tc.want) {
			t.Fatalf("GET %s status=%d missing %q", tc.path, resp.StatusCode, tc.want)
		}
	}
	oaResp, oaDoc := apiJSON(t, http.MethodGet, "/api/v1/openapi.json", "", nil, nil, nil)
	if oaResp.StatusCode != http.StatusOK || oaDoc["components"] == nil {
		t.Fatalf("openapi.json status=%d", oaResp.StatusCode)
	}

	// 2. Signup directly issues session cookie + csrf_token; create Bearer token.
	signupResp, signupDoc := apiJSON(t, http.MethodPost, "/api/v1/auth/signup", "", map[string]any{
		"username": "agentbuyer",
		"email":    "agentbuyer@example.com",
		"password": "AgentPassword123!",
	}, nil, nil)
	if signupResp.StatusCode != http.StatusCreated {
		t.Fatalf("signup status=%d doc=%v", signupResp.StatusCode, signupDoc)
	}
	signupData := signupDoc["data"].(map[string]any)
	csrf, _ := signupData["csrf_token"].(string)
	sessionCookie := getSessionCookie(signupResp)
	if csrf == "" || sessionCookie == nil {
		t.Fatalf("expected session cookie and csrf_token from signup, got csrf=%q cookie=%v", csrf, sessionCookie)
	}

	tokResp, tokDoc := apiJSON(t, http.MethodPost, "/api/v1/me/tokens", "", map[string]any{
		"name":        "e2e-agent",
		"client_type": "agent",
	}, map[string]string{"X-CSRF-Token": csrf}, []*http.Cookie{sessionCookie})
	if tokResp.StatusCode != http.StatusCreated {
		t.Fatalf("create token status=%d doc=%v", tokResp.StatusCode, tokDoc)
	}
	tokData := tokDoc["data"].(map[string]any)
	rawToken := tokData["token"].(string)
	tokenID := int64(tokData["id"].(float64))
	bearer := map[string]string{"Authorization": "Bearer " + rawToken}

	// 3. Catalog traversal with Bearer auth: product exposes store_slug -> /catalog/stores/{slug}.
	prodResp, prodDoc := apiJSON(t, http.MethodGet, "/api/v1/catalog/products/field-compass", "", nil, bearer, nil)
	if prodResp.StatusCode != http.StatusOK {
		t.Fatalf("catalog product status=%d", prodResp.StatusCode)
	}
	storeSlug, _ := prodDoc["data"].(map[string]any)["store_slug"].(string)
	if storeSlug != "agent-goods" {
		t.Fatalf("product store_slug=%q, want agent-goods", storeSlug)
	}
	storeResp, _ := apiJSON(t, http.MethodGet, "/api/v1/catalog/stores/"+storeSlug, "", nil, bearer, nil)
	if storeResp.StatusCode != http.StatusOK {
		t.Fatalf("catalog store status=%d", storeResp.StatusCode)
	}

	// 4. Cart -> checkout -> place-order -> list -> cancel (pure Bearer, no CSRF).
	addResp, _ := apiJSON(t, http.MethodPost, "/api/v1/cart/items", "", map[string]any{
		"product_id": productID,
		"quantity":   1,
	}, bearer, nil)
	if addResp.StatusCode != http.StatusOK {
		t.Fatalf("cart add status=%d", addResp.StatusCode)
	}
	chkResp, chkDoc := apiJSON(t, http.MethodPost, "/api/v1/checkout", "", map[string]any{}, bearer, nil)
	if chkResp.StatusCode != http.StatusCreated {
		t.Fatalf("checkout start status=%d doc=%v", chkResp.StatusCode, chkDoc)
	}
	chkToken := chkDoc["data"].(map[string]any)["token"].(string)

	placeResp, placeDoc := apiJSON(t, http.MethodPost, fmt.Sprintf("/api/v1/checkout/%s/place-order", chkToken), "", map[string]any{
		"delivery_address": "123 Kenyatta Ave, Nairobi",
		"payment_method":   "cash_on_delivery",
	}, bearer, nil)
	if placeResp.StatusCode != http.StatusCreated {
		t.Fatalf("place-order status=%d doc=%v", placeResp.StatusCode, placeDoc)
	}
	orderID := int64(placeDoc["data"].(map[string]any)["order_id"].(float64))

	ordersResp, ordersDoc := apiJSON(t, http.MethodGet, "/api/v1/account/orders", "", nil, bearer, nil)
	if ordersResp.StatusCode != http.StatusOK || int(ordersDoc["data"].(map[string]any)["total"].(float64)) != 1 {
		t.Fatalf("orders status=%d doc=%v", ordersResp.StatusCode, ordersDoc)
	}
	cancelResp, _ := apiJSON(t, http.MethodPost, fmt.Sprintf("/api/v1/account/orders/%d/cancel", orderID), "", map[string]any{}, bearer, nil)
	if cancelResp.StatusCode != http.StatusOK {
		t.Fatalf("cancel order status=%d", cancelResp.StatusCode)
	}

	// 5. Role boundary and token revocation.
	admResp, _ := apiJSON(t, http.MethodGet, "/api/v1/admin/reports", "admin.localhost", nil, bearer, nil)
	if admResp.StatusCode != http.StatusForbidden {
		t.Fatalf("admin reports as customer status=%d, want 403", admResp.StatusCode)
	}
	delResp, _ := apiJSON(t, http.MethodDelete, fmt.Sprintf("/api/v1/me/tokens/%d", tokenID), "", nil, bearer, nil)
	if delResp.StatusCode != http.StatusOK {
		t.Fatalf("revoke token status=%d", delResp.StatusCode)
	}
	afterResp, afterDoc := apiJSON(t, http.MethodGet, "/api/v1/me", "", nil, bearer, nil)
	if afterResp.StatusCode != http.StatusUnauthorized || afterDoc["error"].(map[string]any)["code"] != "invalid_token" {
		t.Fatalf("revoked token status=%d doc=%v", afterResp.StatusCode, afterDoc)
	}
}

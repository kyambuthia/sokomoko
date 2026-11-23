package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kyambuthia/sokomoko/internal/app"
)

func TestOpenAPIEndpoint(t *testing.T) {
	mux := http.NewServeMux()
	Register(app.New(app.Dependencies{Auth: &stubAuthService{}}), mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, apiPrefix+"/openapi.json", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200", rec.Code)
	}
	var doc struct {
		OpenAPI    string                    `json:"openapi"`
		Servers    []map[string]any          `json:"servers"`
		Paths      map[string]map[string]any `json:"paths"`
		Components struct {
			SecuritySchemes map[string]any `json:"securitySchemes"`
			Schemas         map[string]any `json:"schemas"`
			Parameters      map[string]any `json:"parameters"`
			Responses       map[string]any `json:"responses"`
		} `json:"components"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if doc.OpenAPI != "3.1.0" || doc.Paths["/api/v1/auth/login"] == nil || doc.Paths["/api/v1/catalog/categories"] == nil || doc.Paths["/api/v1/catalog/stores"] == nil {
		t.Fatalf("unexpected openapi doc: %+v", doc.OpenAPI)
	}
	if len(doc.Servers) == 0 {
		t.Fatal("expected servers in openapi.json")
	}
	if doc.Components.SecuritySchemes["sessionCookie"] == nil || doc.Components.SecuritySchemes["csrfToken"] == nil {
		t.Fatalf("expected sessionCookie and csrfToken securitySchemes, got %+v", doc.Components.SecuritySchemes)
	}
	for _, key := range []string{"Error", "SignUpRequest", "LoginRequest", "APITokenCreateRequest", "CartItemAddRequest", "CartItemUpdateRequest", "PlaceOrderRequest", "ServiceBookingRequest"} {
		if doc.Components.Schemas[key] == nil {
			t.Fatalf("expected components.schemas.%s", key)
		}
	}
	for _, tc := range []struct {
		path   string
		method string
	}{
		{"/api/v1/auth/signup", "post"},
		{"/api/v1/auth/login", "post"},
		{"/api/v1/me/tokens", "post"},
		{"/api/v1/cart/items", "post"},
		{"/api/v1/cart/items/{id}", "patch"},
		{"/api/v1/checkout/{token}/place-order", "post"},
		{"/api/v1/services/{slug}/bookings", "post"},
	} {
		op := doc.Paths[tc.path][tc.method].(map[string]any)
		if op["requestBody"] == nil {
			t.Fatalf("%s %s missing requestBody", tc.method, tc.path)
		}
	}
	for _, key := range []string{"SlugParam", "IdParam", "TokenParam", "PageParam", "LimitParam"} {
		if doc.Components.Parameters[key] == nil {
			t.Fatalf("missing parameter %s", key)
		}
	}
	for _, key := range []string{"BadRequest", "Unauthorized", "Forbidden", "NotFound", "MethodNotAllowed", "UnsupportedMediaType"} {
		if doc.Components.Responses[key] == nil {
			t.Fatalf("missing response %s", key)
		}
	}
	for path, methods := range doc.Paths {
		for method, rawOp := range methods {
			op, ok := rawOp.(map[string]any)
			if !ok {
				t.Fatalf("%s %s is not an object", method, path)
			}
			resps, ok := op["responses"].(map[string]any)
			if !ok || resps["200"] == nil || resps["400"] == nil || resps["405"] == nil {
				t.Fatalf("%s %s missing standard 200/400/405 responses: %+v", method, path, op["responses"])
			}
		}
	}
}

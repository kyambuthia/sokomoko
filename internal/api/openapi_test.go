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
		OpenAPI string         `json:"openapi"`
		Paths   map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if doc.OpenAPI != "3.1.0" || doc.Paths["/api/v1/auth/login"] == nil || doc.Paths["/api/v1/catalog/categories"] == nil {
		t.Fatalf("unexpected openapi doc: %+v", doc.OpenAPI)
	}
}

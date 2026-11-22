package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kyambuthia/sokomoko/internal/app"
	"github.com/kyambuthia/sokomoko/internal/auth"
	"github.com/kyambuthia/sokomoko/internal/domain"
	"github.com/kyambuthia/sokomoko/internal/money"
	catalogsvc "github.com/kyambuthia/sokomoko/internal/service/catalog"
)

type mockCatalogService struct {
	pageFn          func(ctx context.Context, page, limit int) ([]catalogsvc.Product, int, error)
	searchPageFn    func(ctx context.Context, query, category string, page, limit int) ([]catalogsvc.Product, int, error)
	productBySlugFn func(ctx context.Context, slug string) (*catalogsvc.Product, error)
}

func (m *mockCatalogService) AllProducts(context.Context) ([]catalogsvc.Product, error) {
	return nil, nil
}
func (m *mockCatalogService) Browse(context.Context, catalogsvc.Filter) ([]catalogsvc.Product, error) {
	return nil, nil
}
func (m *mockCatalogService) Categories(context.Context) ([]catalogsvc.Category, error) {
	return nil, nil
}
func (m *mockCatalogService) Page(ctx context.Context, page, limit int) ([]catalogsvc.Product, int, error) {
	if m.pageFn != nil {
		return m.pageFn(ctx, page, limit)
	}
	return nil, 0, nil
}
func (m *mockCatalogService) ProductBySlug(ctx context.Context, slug string) (*catalogsvc.Product, error) {
	if m.productBySlugFn != nil {
		return m.productBySlugFn(ctx, slug)
	}
	return nil, nil
}
func (m *mockCatalogService) ProductImage(context.Context, int) (*catalogsvc.ProductImage, error) {
	return nil, nil
}
func (m *mockCatalogService) ProductVideo(context.Context, int) (*catalogsvc.ProductVideo, error) {
	return nil, nil
}
func (m *mockCatalogService) Search(context.Context, string) ([]catalogsvc.Product, error) {
	return nil, nil
}
func (m *mockCatalogService) SearchInCategory(context.Context, string, string) ([]catalogsvc.Product, error) {
	return nil, nil
}
func (m *mockCatalogService) SearchPage(ctx context.Context, query, category string, page, limit int) ([]catalogsvc.Product, int, error) {
	if m.searchPageFn != nil {
		return m.searchPageFn(ctx, query, category, page, limit)
	}
	return nil, 0, nil
}
func (m *mockCatalogService) ProductCount(context.Context, catalogsvc.Filter) (int, error) {
	return 0, nil
}
func (m *mockCatalogService) ActiveStore(context.Context, string) (*catalogsvc.Partner, error) {
	return nil, nil
}
func (m *mockCatalogService) ActiveStores(context.Context) ([]catalogsvc.Partner, error) {
	return nil, nil
}

func decodeEnvelope[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var env struct {
		Data T `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal response %q: %v", rec.Body.String(), err)
	}
	return env.Data
}

func decodeErrEnvelope(t *testing.T, rec *httptest.ResponseRecorder) errorBody {
	t.Helper()
	var env errorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal error response %q: %v", rec.Body.String(), err)
	}
	return env.Error
}

func TestDecodeJSON(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}

	t.Run("accepts single valid object", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/test", strings.NewReader(`{"name":"soap"}`))
		var out payload
		if err := decodeJSON(req, &out); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.Name != "soap" {
			t.Fatalf("got %q; want soap", out.Name)
		}
	})

	t.Run("rejects unknown fields, arrays, trailing objects, and oversized bodies", func(t *testing.T) {
		cases := []struct {
			name string
			body ioReaderString
		}{
			{name: "unknown field", body: `{"name":"soap","extra":1}`},
			{name: "array instead of object", body: `[{"name":"soap"}]`},
			{name: "trailing object", body: `{"name":"soap"}{"name":"two"}`},
			{name: "empty body", body: ``},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				req := httptest.NewRequest(http.MethodPost, "/api/v1/test", strings.NewReader(string(tc.body)))
				var out payload
				if err := decodeJSON(req, &out); err == nil {
					t.Fatalf("expected error for %s", tc.name)
				}
			})
		}

		oversized := `{"name":"` + strings.Repeat("a", maxJSONBodyBytes+10) + `"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/test", bytes.NewBufferString(oversized))
		var out payload
		if err := decodeJSON(req, &out); err == nil {
			t.Fatal("expected error for oversized body")
		}
	})
}

type ioReaderString string

func TestHelpers(t *testing.T) {
	if id, ok := parsePositiveID(" 42 "); !ok || id != 42 {
		t.Fatalf("parsePositiveID(\" 42 \") = %d, %v; want 42, true", id, ok)
	}
	for _, raw := range []string{"0", "-3", "abc", ""} {
		if _, ok := parsePositiveID(raw); ok {
			t.Fatalf("parsePositiveID(%q) should fail", raw)
		}
	}
	if got := pathTail("/api/v1/catalog/products/blue-lamp/", apiPrefix+"/catalog/products/"); got != "blue-lamp" {
		t.Fatalf("pathTail = %q; want blue-lamp", got)
	}
}

func TestHandlerBasicResponses(t *testing.T) {
	h := &Handler{app: app.New(app.Dependencies{})}

	t.Run("notFound returns 404 JSON error with no-store", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/unknown", nil)
		h.notFound(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d; want 404", rec.Code)
		}
		if got := rec.Header().Get("Cache-Control"); got != "no-store" {
			t.Fatalf("Cache-Control = %q; want no-store", got)
		}
		if errBody := decodeErrEnvelope(t, rec); errBody.Code != "not_found" {
			t.Fatalf("error code = %q; want not_found", errBody.Code)
		}
	})

	t.Run("demoCheckoutDisabled returns 403", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/checkout", nil)
		h.demoCheckoutDisabled(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d; want 403", rec.Code)
		}
		if errBody := decodeErrEnvelope(t, rec); errBody.Code != "demo_mode" {
			t.Fatalf("error code = %q; want demo_mode", errBody.Code)
		}
	})
}

func TestCatalogEndpoints(t *testing.T) {
	t.Run("catalogProducts enforces GET and clamps pagination", func(t *testing.T) {
		var gotPage, gotLimit int
		h := &Handler{app: app.New(app.Dependencies{
			Catalog: &mockCatalogService{
				pageFn: func(_ context.Context, page, limit int) ([]catalogsvc.Product, int, error) {
					gotPage, gotLimit = page, limit
					return []catalogsvc.Product{{
						ID: 7, Name: "Brass Lamp", Slug: "brass-lamp",
						Price: money.Cents(1500), Currency: "KES", StockQuantity: 3,
					}}, 1, nil
				},
			},
		})}

		// POST rejected
		postRec := httptest.NewRecorder()
		h.catalogProducts(postRec, httptest.NewRequest(http.MethodPost, apiPrefix+"/catalog/products", nil))
		if postRec.Code != http.StatusMethodNotAllowed || postRec.Header().Get("Allow") != "GET, HEAD" {
			t.Fatalf("POST status=%d allow=%q", postRec.Code, postRec.Header().Get("Allow"))
		}

		// GET clamps page=0 -> 1 and limit=200 -> 50
		rec := httptest.NewRecorder()
		h.catalogProducts(rec, httptest.NewRequest(http.MethodGet, apiPrefix+"/catalog/products?page=0&limit=200", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d; want 200", rec.Code)
		}
		if gotPage != 1 || gotLimit != maxPageLimit {
			t.Fatalf("clamped page=%d limit=%d; want 1, %d", gotPage, gotLimit, maxPageLimit)
		}
		data := decodeEnvelope[catalogListDTO](t, rec)
		if data.Total != 1 || len(data.Items) != 1 || data.Items[0].Slug != "brass-lamp" || data.Items[0].PriceMinor != 1500 {
			t.Fatalf("unexpected catalog list DTO: %+v", data)
		}
	})

	t.Run("catalogSearch short-circuits empty query and handles errors", func(t *testing.T) {
		h := &Handler{app: app.New(app.Dependencies{
			Catalog: &mockCatalogService{
				searchPageFn: func(context.Context, string, string, int, int) ([]catalogsvc.Product, int, error) {
					return nil, 0, errors.New("db down")
				},
			},
		})}

		emptyRec := httptest.NewRecorder()
		h.catalogSearch(emptyRec, httptest.NewRequest(http.MethodGet, apiPrefix+"/catalog/search", nil))
		if emptyRec.Code != http.StatusOK {
			t.Fatalf("empty search status = %d; want 200", emptyRec.Code)
		}
		if data := decodeEnvelope[catalogListDTO](t, emptyRec); len(data.Items) != 0 || data.Total != 0 {
			t.Fatalf("expected empty list, got %+v", data)
		}

		errRec := httptest.NewRecorder()
		h.catalogSearch(errRec, httptest.NewRequest(http.MethodGet, apiPrefix+"/catalog/search?q=lamp", nil))
		if errRec.Code != http.StatusInternalServerError {
			t.Fatalf("error search status = %d; want 500", errRec.Code)
		}
		if errBody := decodeErrEnvelope(t, errRec); errBody.Code != "search_unavailable" {
			t.Fatalf("error code = %q; want search_unavailable", errBody.Code)
		}

		for _, rawURL := range []string{
			apiPrefix + "/catalog/search?q=%00",
			apiPrefix + "/catalog/search?q=lamp%00",
			apiPrefix + "/catalog/search?q=%FF",
			apiPrefix + "/catalog/search?q=%C3%28",
			apiPrefix + "/catalog/search?category=%00",
			apiPrefix + "/catalog/search?category=%FF",
		} {
			badRec := httptest.NewRecorder()
			h.catalogSearch(badRec, httptest.NewRequest(http.MethodGet, rawURL, nil))
			if badRec.Code != http.StatusBadRequest {
				t.Fatalf("%s status = %d; want 400", rawURL, badRec.Code)
			}
			if errBody := decodeErrEnvelope(t, badRec); errBody.Code != "invalid_query" {
				t.Fatalf("%s error code = %q; want invalid_query", rawURL, errBody.Code)
			}
		}
	})

	t.Run("catalogProduct maps invalid slug, missing, and valid product", func(t *testing.T) {
		h := &Handler{app: app.New(app.Dependencies{
			Catalog: &mockCatalogService{
				productBySlugFn: func(_ context.Context, slug string) (*catalogsvc.Product, error) {
					switch slug {
					case "bad/slug":
						return nil, catalogsvc.ErrInvalidProductSlug
					case "missing":
						return nil, nil
					case "tea":
						return &catalogsvc.Product{ID: 1, Name: "Tea", Slug: "tea", Price: 500, Currency: "KES"}, nil
					default:
						return nil, errors.New("boom")
					}
				},
			},
		})}

		// Bad slug -> 400
		badRec := httptest.NewRecorder()
		h.catalogProduct(badRec, httptest.NewRequest(http.MethodGet, apiPrefix+"/catalog/products/bad/slug", nil))
		if badRec.Code != http.StatusBadRequest {
			t.Fatalf("bad slug status = %d; want 400", badRec.Code)
		}

		// Missing -> 404
		missRec := httptest.NewRecorder()
		h.catalogProduct(missRec, httptest.NewRequest(http.MethodGet, apiPrefix+"/catalog/products/missing", nil))
		if missRec.Code != http.StatusNotFound {
			t.Fatalf("missing status = %d; want 404", missRec.Code)
		}

		// Valid -> 200
		okRec := httptest.NewRecorder()
		h.catalogProduct(okRec, httptest.NewRequest(http.MethodGet, apiPrefix+"/catalog/products/tea", nil))
		if okRec.Code != http.StatusOK {
			t.Fatalf("ok status = %d; want 200", okRec.Code)
		}
		if dto := decodeEnvelope[productDTO](t, okRec); dto.Slug != "tea" || dto.PriceMinor != 500 {
			t.Fatalf("unexpected product DTO: %+v", dto)
		}
	})
}

func TestDemoMode_CheckoutRequiresAuthAndWriteGuardBlocksMutations(t *testing.T) {
	h := &Handler{app: &app.App{DemoMode: true, Auth: &auth.Service{}}}

	mux := http.NewServeMux()
	Register(h.app, mux)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, apiPrefix+"/checkout", nil)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated demo checkout status = %d; want 401", rec.Code)
	}

	called := false
	guarded := h.demoWriteDisabled(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	getRec := httptest.NewRecorder()
	guarded.ServeHTTP(getRec, httptest.NewRequest(http.MethodGet, apiPrefix+"/partner/services", nil))
	if !called || getRec.Code != http.StatusOK {
		t.Fatalf("GET demoWriteDisabled called=%v status=%d; want true/200", called, getRec.Code)
	}

	called = false
	postRec := httptest.NewRecorder()
	guarded.ServeHTTP(postRec, httptest.NewRequest(http.MethodPost, apiPrefix+"/partner/services", nil))
	if called || postRec.Code != http.StatusForbidden {
		t.Fatalf("POST demoWriteDisabled called=%v status=%d; want false/403", called, postRec.Code)
	}
}

func TestRequireJSONContentType(t *testing.T) {
	mux := http.NewServeMux()
	Register(app.New(app.Dependencies{
		Auth: &stubAuthService{user: &auth.User{ID: 1, Username: "alice", Email: "a@b.c", Role: domain.RoleUser}},
	}), mux)

	t.Run("rejects text/plain body with 415", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, apiPrefix+"/auth/login", strings.NewReader(`{"username":"a","password":"b"}`))
		req.Header.Set("Content-Type", "text/plain")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnsupportedMediaType {
			t.Fatalf("status = %d, want 415; body=%s", rec.Code, rec.Body.String())
		}
		if errBody := decodeErrEnvelope(t, rec); errBody.Code != "unsupported_media_type" {
			t.Fatalf("error code = %q, want unsupported_media_type", errBody.Code)
		}
	})

	t.Run("rejects missing Content-Type when body is sent with 415", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPatch, apiPrefix+"/me", strings.NewReader(`{"username":"bob","email":"b@b.c"}`))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnsupportedMediaType {
			t.Fatalf("status = %d, want 415; body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("allows application/json with charset=utf-8 and bodyless POST", func(t *testing.T) {
		logoutMux := http.NewServeMux()
		Register(app.New(app.Dependencies{
			Auth: &mockAPIAuth{
				stubAuthService: stubAuthService{user: &auth.User{ID: 1, Username: "alice", Email: "a@b.c", Role: domain.RoleUser}},
			},
		}), logoutMux)
		req := httptest.NewRequest(http.MethodPost, apiPrefix+"/auth/logout", nil)
		rec := httptest.NewRecorder()
		logoutMux.ServeHTTP(rec, req)
		if rec.Code == http.StatusUnsupportedMediaType {
			t.Fatalf("bodyless POST should not return 415")
		}

		h := &Handler{app: &app.App{}}
		called := false
		wrapped := h.requireJSON(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
		}))
		reqJSON := httptest.NewRequest(http.MethodPatch, apiPrefix+"/me", strings.NewReader(`{"username":"bob"}`))
		reqJSON.Header.Set("Content-Type", "application/json; charset=utf-8")
		recJSON := httptest.NewRecorder()
		wrapped.ServeHTTP(recJSON, reqJSON)
		if !called || recJSON.Code != http.StatusOK {
			t.Fatalf("called=%v status=%d, want true/200", called, recJSON.Code)
		}
	})
}

func TestDemoMode_PatchMeChecksAuthThenDemoBeforeValidation(t *testing.T) {
	unauthMux := http.NewServeMux()
	Register(app.New(app.Dependencies{DemoMode: true, Auth: &auth.Service{}}), unauthMux)
	unauthReq := httptest.NewRequest(http.MethodPatch, apiPrefix+"/me", strings.NewReader(`{"username":""}`))
	unauthReq.Header.Set("Content-Type", "application/json")
	unauthRec := httptest.NewRecorder()
	unauthMux.ServeHTTP(unauthRec, unauthReq)
	if unauthRec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated PATCH /api/v1/me status = %d, want 401", unauthRec.Code)
	}

	authMux := http.NewServeMux()
	Register(app.New(app.Dependencies{
		DemoMode: true,
		Auth:     &stubAuthService{user: &auth.User{ID: 1, Username: "demo", Email: "demo@example.com", Role: domain.RoleUser}},
	}), authMux)
	authReq := httptest.NewRequest(http.MethodPatch, apiPrefix+"/me", strings.NewReader(`{"username":"","email":"bad"}`))
	authReq.Header.Set("Content-Type", "application/json")
	authRec := httptest.NewRecorder()
	authMux.ServeHTTP(authRec, authReq)
	if authRec.Code != http.StatusForbidden {
		t.Fatalf("authenticated demo PATCH /api/v1/me status = %d, want 403; body=%s", authRec.Code, authRec.Body.String())
	}
	if errBody := decodeErrEnvelope(t, authRec); errBody.Code != "demo_mode" {
		t.Fatalf("error code = %q, want demo_mode", errBody.Code)
	}
}

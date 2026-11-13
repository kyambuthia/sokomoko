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
	"github.com/kyambuthia/sokomoko/internal/db"
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
func (m *mockCatalogService) ProductImage(context.Context, int) (*db.ProductImage, error) {
	return nil, nil
}
func (m *mockCatalogService) ProductVideo(context.Context, int) (*db.ProductVideo, error) {
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
func (m *mockCatalogService) ActiveStore(context.Context, string) (*db.Partner, error) {
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
		if postRec.Code != http.StatusMethodNotAllowed || postRec.Header().Get("Allow") != http.MethodGet {
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

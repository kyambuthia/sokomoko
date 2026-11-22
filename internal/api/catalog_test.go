package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kyambuthia/sokomoko/internal/app"
	catalogsvc "github.com/kyambuthia/sokomoko/internal/service/catalog"
)

type stubCatalogPlus struct {
	mockCatalogService
	categoriesFn  func(context.Context) ([]catalogsvc.Category, error)
	activeStoreFn func(context.Context, string) (*catalogsvc.Partner, error)
	browseFn      func(context.Context, catalogsvc.Filter) ([]catalogsvc.Product, error)
	countFn       func(context.Context, catalogsvc.Filter) (int, error)
}

func (s *stubCatalogPlus) Categories(ctx context.Context) ([]catalogsvc.Category, error) {
	return s.categoriesFn(ctx)
}
func (s *stubCatalogPlus) ActiveStore(ctx context.Context, slug string) (*catalogsvc.Partner, error) {
	return s.activeStoreFn(ctx, slug)
}
func (s *stubCatalogPlus) Browse(ctx context.Context, f catalogsvc.Filter) ([]catalogsvc.Product, error) {
	return s.browseFn(ctx, f)
}
func (s *stubCatalogPlus) ProductCount(ctx context.Context, f catalogsvc.Filter) (int, error) {
	return s.countFn(ctx, f)
}

func TestCatalogCategoriesAndStoreEndpoints(t *testing.T) {
	catSvc := &stubCatalogPlus{
		categoriesFn: func(context.Context) ([]catalogsvc.Category, error) {
			return []catalogsvc.Category{{Name: "Electronics", Slug: "electronics"}}, nil
		},
		activeStoreFn: func(_ context.Context, slug string) (*catalogsvc.Partner, error) {
			if slug == "urban-goods" {
				return &catalogsvc.Partner{ID: 1, Name: "Urban Goods", Slug: "urban-goods"}, nil
			}
			return nil, nil
		},
		browseFn: func(context.Context, catalogsvc.Filter) ([]catalogsvc.Product, error) {
			return []catalogsvc.Product{{ID: 1, Name: "Lamp", Slug: "lamp"}}, nil
		},
		countFn: func(context.Context, catalogsvc.Filter) (int, error) { return 1, nil },
	}
	mux := http.NewServeMux()
	Register(app.New(app.Dependencies{Catalog: catSvc, Auth: &stubAuthService{}}), mux)

	catRec := httptest.NewRecorder()
	mux.ServeHTTP(catRec, httptest.NewRequest(http.MethodGet, apiPrefix+"/catalog/categories", nil))
	if catRec.Code != http.StatusOK || !strings.Contains(catRec.Body.String(), "electronics") {
		t.Fatalf("categories status=%d body=%s", catRec.Code, catRec.Body.String())
	}

	storeRec := httptest.NewRecorder()
	mux.ServeHTTP(storeRec, httptest.NewRequest(http.MethodGet, apiPrefix+"/catalog/stores/urban-goods", nil))
	if storeRec.Code != http.StatusOK || !strings.Contains(storeRec.Body.String(), "Urban Goods") {
		t.Fatalf("store status=%d body=%s", storeRec.Code, storeRec.Body.String())
	}

	missingRec := httptest.NewRecorder()
	mux.ServeHTTP(missingRec, httptest.NewRequest(http.MethodGet, apiPrefix+"/catalog/stores/nope", nil))
	if missingRec.Code != http.StatusNotFound {
		t.Fatalf("missing store status=%d, want 404", missingRec.Code)
	}

	for _, path := range []string{
		apiPrefix + "/catalog/products",
		apiPrefix + "/catalog/search",
		apiPrefix + "/catalog/categories",
		apiPrefix + "/catalog/stores/urban-goods",
	} {
		headRec := httptest.NewRecorder()
		mux.ServeHTTP(headRec, httptest.NewRequest(http.MethodHead, path, nil))
		if headRec.Code != http.StatusOK {
			t.Fatalf("HEAD %s status=%d, want 200", path, headRec.Code)
		}
		if headRec.Body.Len() != 0 {
			t.Fatalf("HEAD %s wrote non-empty body: %q", path, headRec.Body.String())
		}
		if ct := headRec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
			t.Fatalf("HEAD %s Content-Type=%q, want application/json", path, ct)
		}

		postRec := httptest.NewRecorder()
		mux.ServeHTTP(postRec, httptest.NewRequest(http.MethodPost, path, nil))
		if postRec.Code != http.StatusMethodNotAllowed || postRec.Header().Get("Allow") != "GET, HEAD" {
			t.Fatalf("POST %s status=%d allow=%q, want 405 and GET, HEAD", path, postRec.Code, postRec.Header().Get("Allow"))
		}
	}
}

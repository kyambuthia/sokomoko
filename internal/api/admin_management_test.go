package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kyambuthia/sokomoko/internal/app"
	"github.com/kyambuthia/sokomoko/internal/auth"
	"github.com/kyambuthia/sokomoko/internal/domain"
	adminsvc "github.com/kyambuthia/sokomoko/internal/service/admin"
)

type stubAdminMgmt struct {
	app.AdminService
	categoriesFn func(context.Context) ([]adminsvc.Category, error)
	createCatFn  func(context.Context, *adminsvc.Actor, adminsvc.CategoryInput) error
	partnersFn   func(context.Context) ([]adminsvc.Partner, []adminsvc.PartnerAccount, error)
	reactivateFn func(context.Context, *adminsvc.Actor, string) error
}

func (s *stubAdminMgmt) Categories(ctx context.Context) ([]adminsvc.Category, error) {
	return s.categoriesFn(ctx)
}
func (s *stubAdminMgmt) CreateCategory(ctx context.Context, a *adminsvc.Actor, in adminsvc.CategoryInput) error {
	return s.createCatFn(ctx, a, in)
}
func (s *stubAdminMgmt) Partners(ctx context.Context) ([]adminsvc.Partner, []adminsvc.PartnerAccount, error) {
	return s.partnersFn(ctx)
}
func (s *stubAdminMgmt) ReactivateUser(ctx context.Context, a *adminsvc.Actor, id string) error {
	return s.reactivateFn(ctx, a, id)
}

func TestAdminManagementEndpoints(t *testing.T) {
	adm := &stubAdminMgmt{
		categoriesFn: func(context.Context) ([]adminsvc.Category, error) {
			return []adminsvc.Category{{ID: 1, Name: "Home", Slug: "home"}}, nil
		},
		createCatFn: func(context.Context, *adminsvc.Actor, adminsvc.CategoryInput) error { return nil },
		partnersFn: func(context.Context) ([]adminsvc.Partner, []adminsvc.PartnerAccount, error) {
			return []adminsvc.Partner{{ID: 1, Name: "Urban Goods", Slug: "urban-goods"}}, nil, nil
		},
		reactivateFn: func(context.Context, *adminsvc.Actor, string) error { return nil },
	}
	mux := http.NewServeMux()
	Register(app.New(app.Dependencies{
		Auth:  &stubAuthService{user: &auth.User{ID: 1, Role: domain.RoleAdmin}},
		Admin: adm,
	}), mux)

	catGet := httptest.NewRecorder()
	mux.ServeHTTP(catGet, httptest.NewRequest(http.MethodGet, apiPrefix+"/admin/categories", nil))
	if catGet.Code != http.StatusOK || !strings.Contains(catGet.Body.String(), "home") {
		t.Fatalf("GET categories=%d body=%s", catGet.Code, catGet.Body.String())
	}

	catPost := httptest.NewRecorder()
	mux.ServeHTTP(catPost, httptest.NewRequest(http.MethodPost, apiPrefix+"/admin/categories", strings.NewReader(`{"name":"Tech","slug":"tech"}`)))
	if catPost.Code != http.StatusCreated {
		t.Fatalf("POST categories=%d body=%s", catPost.Code, catPost.Body.String())
	}

	partGet := httptest.NewRecorder()
	mux.ServeHTTP(partGet, httptest.NewRequest(http.MethodGet, apiPrefix+"/admin/partners", nil))
	if partGet.Code != http.StatusOK || !strings.Contains(partGet.Body.String(), "Urban Goods") {
		t.Fatalf("GET partners=%d body=%s", partGet.Code, partGet.Body.String())
	}

	reactPost := httptest.NewRecorder()
	mux.ServeHTTP(reactPost, httptest.NewRequest(http.MethodPost, apiPrefix+"/admin/team/9/reactivate", nil))
	if reactPost.Code != http.StatusOK {
		t.Fatalf("POST reactivate=%d body=%s", reactPost.Code, reactPost.Body.String())
	}
}

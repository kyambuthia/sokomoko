package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kyambuthia/sokomoko/internal/app"
	"github.com/kyambuthia/sokomoko/internal/auth"
	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/domain"
)

func TestDemoReadOnly_AllowsGetAndUnauthenticatedAndBlocksAuthenticatedMutations(t *testing.T) {
	a := &app.App{DemoMode: true}
	called := false
	handler := demoReadOnly(a, "/orders", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	getReq := httptest.NewRequest(http.MethodGet, "/orders", nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if !called || getRec.Code != http.StatusOK {
		t.Fatalf("GET called=%v code=%d, want true/200", called, getRec.Code)
	}

	called = false
	unauthPost := httptest.NewRequest(http.MethodPost, "/orders", nil)
	unauthRec := httptest.NewRecorder()
	handler.ServeHTTP(unauthRec, unauthPost)
	if !called || unauthRec.Code != http.StatusOK {
		t.Fatalf("unauthenticated POST should pass through to auth, called=%v code=%d", called, unauthRec.Code)
	}

	called = false
	postReq := httptest.NewRequest(http.MethodPost, "/orders", nil)
	postReq = postReq.WithContext(auth.ContextWithUser(postReq.Context(), &db.User{ID: 1, Role: "partner"}))
	postRec := httptest.NewRecorder()
	handler.ServeHTTP(postRec, postReq)
	if called {
		t.Fatal("authenticated POST reached wrapped handler in demo mode")
	}
	if postRec.Code != http.StatusFound || postRec.Header().Get("Location") != "/orders" {
		t.Fatalf("POST code=%d location=%q, want 302 /orders", postRec.Code, postRec.Header().Get("Location"))
	}
}

func TestDemoReadOnly_InsideRoleMiddlewareBlocksAuthenticatedPost(t *testing.T) {
	a := &app.App{DemoMode: true}
	called := false
	handler := auth.RequireRole(domain.RolePartner, demoReadOnly(a, "/products", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})))

	req := httptest.NewRequest(http.MethodPost, "/products/1/delete", nil)
	req = req.WithContext(auth.ContextWithUser(req.Context(), &db.User{ID: 2, Role: domain.RolePartner}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if called {
		t.Fatal("authenticated partner POST reached handler in demo mode")
	}
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/products" {
		t.Fatalf("POST code=%d location=%q, want 302 /products", rec.Code, rec.Header().Get("Location"))
	}
}

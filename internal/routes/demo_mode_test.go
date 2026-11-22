package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kyambuthia/sokomoko/internal/app"
)

func TestDemoReadOnly_AllowsGetAndBlocksMutations(t *testing.T) {
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
	postReq := httptest.NewRequest(http.MethodPost, "/orders", nil)
	postRec := httptest.NewRecorder()
	handler.ServeHTTP(postRec, postReq)
	if called {
		t.Fatal("POST reached wrapped handler in demo mode")
	}
	if postRec.Code != http.StatusFound || postRec.Header().Get("Location") != "/orders" {
		t.Fatalf("POST code=%d location=%q, want 302 /orders", postRec.Code, postRec.Header().Get("Location"))
	}
}

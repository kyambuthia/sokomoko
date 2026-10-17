package routes

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSiblingURL(t *testing.T) {
	cases := []struct {
		host, prefix, path, want string
		https                    bool
	}{
		{"admin.localhost:6969", "partner.", "/products", "http://partner.localhost:6969/products", false},
		{"partner.shop.example", "", "/", "https://shop.example/", true},
		{"shop.example", "admin.", "/orders", "http://admin.shop.example/orders", false},
		{"ADMIN.Shop.Example", "", "/", "http://shop.example/", false},
	}
	for _, tc := range cases {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Host = tc.host
		if tc.https {
			r.TLS = &tls.ConnectionState{}
		}
		if got := siblingURL(r, tc.prefix, tc.path); got != tc.want {
			t.Errorf("siblingURL(%q, %q, %q) = %q, want %q", tc.host, tc.prefix, tc.path, got, tc.want)
		}
	}
}

func TestRedirectToSibling(t *testing.T) {
	h := redirectToSibling(partnerHostPrefix, "/products/new")
	r := httptest.NewRequest(http.MethodGet, "/catalog/new", nil)
	r.Host = "admin.localhost:6969"
	rec := httptest.NewRecorder()
	h(rec, r)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "http://partner.localhost:6969/products/new" {
		t.Fatalf("got %d %q", rec.Code, rec.Header().Get("Location"))
	}

	rec = httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodPost, "/catalog/new", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST got %d, want 405", rec.Code)
	}
}

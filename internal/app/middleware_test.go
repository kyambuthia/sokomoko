package app

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCanonicalHost(t *testing.T) {
	if got := CanonicalHost("Admin.Localhost:6969"); got != "admin.localhost" {
		t.Fatalf("expected admin.localhost, got %q", got)
	}
}

func TestCSRFSameOriginBlocksCrossOriginWithSession(t *testing.T) {
	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), CSRFSameOrigin("session_token"))

	req := httptest.NewRequest(http.MethodPost, "http://admin.localhost/team", strings.NewReader("x=1"))
	req.AddCookie(&http.Cookie{Name: "session_token", Value: "abc"})
	req.Header.Set("Origin", "http://evil.local")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected %d, got %d", http.StatusForbidden, rec.Code)
	}
}

func TestCSRFSameOriginAllowsSameOriginWithSession(t *testing.T) {
	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), CSRFSameOrigin("session_token"))

	req := httptest.NewRequest(http.MethodPost, "http://admin.localhost/team", strings.NewReader("x=1"))
	req.AddCookie(&http.Cookie{Name: "session_token", Value: "abc"})
	req.Header.Set("Origin", "http://admin.localhost")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected %d, got %d", http.StatusNoContent, rec.Code)
	}
}

func TestCSRFSameOriginBlocksDifferentPortWithSession(t *testing.T) {
	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), CSRFSameOrigin("session_token"))

	req := httptest.NewRequest(http.MethodPost, "http://admin.localhost:6969/team", strings.NewReader("x=1"))
	req.AddCookie(&http.Cookie{Name: "session_token", Value: "abc"})
	req.Header.Set("Origin", "http://admin.localhost:3000")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected %d, got %d", http.StatusForbidden, rec.Code)
	}
}

func TestCSRFSameOriginBlocksDifferentSchemeWithSession(t *testing.T) {
	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), CSRFSameOrigin("session_token"))

	req := httptest.NewRequest(http.MethodPost, "https://admin.localhost/team", strings.NewReader("x=1"))
	req.AddCookie(&http.Cookie{Name: "session_token", Value: "abc"})
	req.Header.Set("Origin", "http://admin.localhost")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected %d, got %d", http.StatusForbidden, rec.Code)
	}
}

func TestCSRFSameOriginBlocksMissingOriginAndRefererWithSession(t *testing.T) {
	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), CSRFSameOrigin("session_token"))

	req := httptest.NewRequest(http.MethodPost, "http://admin.localhost/team", strings.NewReader("x=1"))
	req.AddCookie(&http.Cookie{Name: "session_token", Value: "abc"})
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected %d, got %d", http.StatusForbidden, rec.Code)
	}
}

func TestSecurityHeaders(t *testing.T) {
	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), SecurityHeaders())

	req := httptest.NewRequest(http.MethodGet, "http://localhost/", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("expected nosniff header, got %q", got)
	}
	if got := rec.Header().Get("Content-Security-Policy"); got == "" {
		t.Fatal("expected content security policy header")
	}
}

func TestBodyLimit(t *testing.T) {
	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}), BodyLimit(4))

	req := httptest.NewRequest(http.MethodPost, "http://localhost/", strings.NewReader("12345"))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected %d, got %d", http.StatusRequestEntityTooLarge, rec.Code)
	}
}

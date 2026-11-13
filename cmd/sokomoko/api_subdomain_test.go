package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBuildHostRouter_APISubdomain(t *testing.T) {
	allowed := map[string]struct{}{
		"localhost":     {},
		"api.localhost": {},
	}
	mainMux := http.NewServeMux()
	mainMux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("storefront"))
	})
	mainMux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("ok"))
	})
	apiMux := http.NewServeMux()
	apiMux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": "not_found"}})
	})
	apiMux.HandleFunc("/api/v1/ping", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("pong"))
	})

	router := buildHostRouter(allowed, mainMux, http.NewServeMux(), http.NewServeMux(), http.NewServeMux(), http.NewServeMux(), apiMux, http.NotFoundHandler())

	// api.localhost serves /api/v1/* and returns JSON 404 for unknown paths (not storefront HTML).
	apiReq := httptest.NewRequest(http.MethodGet, "/api/v1/ping", nil)
	apiReq.Host = "api.localhost"
	apiRec := httptest.NewRecorder()
	router.ServeHTTP(apiRec, apiReq)
	if apiRec.Code != http.StatusOK || apiRec.Body.String() != "pong" {
		t.Fatalf("api ping = %d %q; want 200 pong", apiRec.Code, apiRec.Body.String())
	}

	rootReq := httptest.NewRequest(http.MethodGet, "/", nil)
	rootReq.Host = "api.localhost"
	rootRec := httptest.NewRecorder()
	router.ServeHTTP(rootRec, rootReq)
	if rootRec.Code != http.StatusNotFound || rootRec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("api root = %d %q; want 404 application/json", rootRec.Code, rootRec.Header().Get("Content-Type"))
	}

	// /healthz still works on api.localhost.
	hzReq := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	hzReq.Host = "api.localhost"
	hzRec := httptest.NewRecorder()
	router.ServeHTTP(hzRec, hzReq)
	if hzRec.Code != http.StatusOK || hzRec.Body.String() != "ok" {
		t.Fatalf("api healthz = %d %q; want 200 ok", hzRec.Code, hzRec.Body.String())
	}
}

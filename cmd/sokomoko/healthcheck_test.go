package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheckReadiness_HostAndStatus(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusServiceUnavailable, http.StatusFound} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Host != "shop.example.com" || r.URL.Path != "/readyz" {
				t.Errorf("unexpected probe: host=%s path=%s", r.Host, r.URL.Path)
			}
			w.WriteHeader(status)
		}))
		err := checkReadiness(context.Background(), server.URL+"/readyz", "shop.example.com")
		server.Close()
		if (err == nil) != (status == http.StatusOK) {
			t.Fatalf("status=%d error=%v", status, err)
		}
	}
}

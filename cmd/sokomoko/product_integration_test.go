package main

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestIntegration_ProductDetailRoute(t *testing.T) {
	clearAllTables()

	suffix := time.Now().UnixNano()
	slug := fmt.Sprintf("detail-product-%d", suffix)
	name := "Detail Product"
	createTestProduct(t, name, slug, 25.0, 4)

	resp, body := makeRequest(http.MethodGet, "/products/"+slug, nil, nil, "")
	if resp == nil {
		t.Fatal("request failed")
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /products/%s status=%d expected=%d", slug, resp.StatusCode, http.StatusOK)
	}
	if !strings.Contains(body, name) {
		t.Fatalf("expected product name %q in detail page", name)
	}
}

func TestIntegration_ProductDetailRoute_NotFound(t *testing.T) {
	clearAllTables()

	resp, _ := makeRequest(http.MethodGet, "/products/does-not-exist", nil, nil, "")
	if resp == nil {
		t.Fatal("request failed")
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET missing product detail status=%d expected=%d", resp.StatusCode, http.StatusNotFound)
	}
}

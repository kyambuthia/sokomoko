package main

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/kyambuthia/sokomoko/internal/db"
)

func TestCourierAPIStatusRetriesPreserveAuthorizationAndTimeline(t *testing.T) {
	clearAllTables()
	adminID, admin := messageTestSession(t, "retry-dispatch", db.RoleAdmin, "admin.localhost")
	buyerID, buyer := messageTestSession(t, "retry-buyer", db.RoleUser, "")
	riderID, rider := messageTestSession(t, "retry-rider", db.RoleCourier, "courier.localhost")
	_, outsider := messageTestSession(t, "retry-outsider", db.RoleCourier, "courier.localhost")
	product := createTestProduct(t, "Retry delivery fixture", "retry-delivery-fixture", 100, 3)
	if err := testStore.AddToCart(t.Context(), buyerID, int(product), 1); err != nil {
		t.Fatal(err)
	}
	order, err := testStore.PlaceOrderFromCart(t.Context(), buyerID, "Sample test address")
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range []string{"accepted", "packing", "dispatched"} {
		if err := testStore.UpdateOrderFulfillment(t.Context(), int(order), step, "queued", ""); err != nil {
			t.Fatal(err)
		}
	}
	if err := testStore.AssignCourierToOrder(t.Context(), int(order), riderID, adminID); err != nil {
		t.Fatal(err)
	}
	boardResponse, boardBody := makeAPIRequestOnHost(t, "admin.localhost", http.MethodGet, "/api/v1/admin/deliveries", nil, admin, false, nil)
	messagingStatus(t, boardResponse, boardBody, http.StatusOK)
	var board struct {
		Couriers []struct {
			ID     int `json:"id"`
			Active int `json:"active_deliveries"`
			Failed int `json:"failed_deliveries"`
		} `json:"couriers"`
	}
	decodeAPIData(t, boardBody, &board)
	found := false
	for _, courier := range board.Couriers {
		if courier.ID == riderID {
			found = true
			if courier.Active != 1 || courier.Failed != 0 {
				t.Fatalf("courier workload=%+v", courier)
			}
		}
	}
	if !found {
		t.Fatal("assigned courier missing from dispatch API")
	}
	boardPageResponse, boardPage := makeRequest(http.MethodGet, "/deliveries", nil, admin, "admin.localhost")
	if boardPageResponse.StatusCode != http.StatusOK || !strings.Contains(boardPage, "retry-rider · 1 active · 0 failed") {
		t.Fatal("dispatch HTML workload missing")
	}
	jobs, err := testStore.ListCourierDeliveries(t.Context(), riderID)
	if err != nil || len(jobs) != 1 {
		t.Fatalf("jobs=%+v, %v", jobs, err)
	}
	respBefore, bodyBefore := makeAPIRequestOnHost(t, "courier.localhost", http.MethodGet, "/api/v1/courier/deliveries", nil, rider, false, nil)
	messagingStatus(t, respBefore, bodyBefore, http.StatusOK)
	if !strings.Contains(string(bodyBefore), "Sample test address") {
		t.Fatal("active delivery lost its address")
	}
	assertView := func(view string, want int) {
		t.Helper()
		response, raw := makeAPIRequestOnHost(t, "courier.localhost", http.MethodGet, "/api/v1/courier/deliveries?view="+view, nil, rider, false, nil)
		messagingStatus(t, response, raw, http.StatusOK)
		var selected []struct {
			OrderID int `json:"order_id"`
		}
		decodeAPIData(t, raw, &selected)
		if len(selected) != want || (want == 1 && selected[0].OrderID != int(order)) {
			t.Fatalf("view %s: %+v", view, selected)
		}
	}
	assertView("active", 1)
	assertView("completed", 0)
	invalidResponse, invalidBody := makeAPIRequestOnHost(t, "courier.localhost", http.MethodGet, "/api/v1/courier/deliveries?view=invalid", nil, rider, false, nil)
	messagingStatus(t, invalidResponse, invalidBody, http.StatusBadRequest)
	invalidPage, _ := makeRequest(http.MethodGet, "/deliveries?view=invalid", nil, rider, "courier.localhost")
	if invalidPage.StatusCode != http.StatusBadRequest {
		t.Fatal("invalid HTML view accepted")
	}
	path := fmt.Sprintf("/api/v1/courier/deliveries/%d", jobs[0].ID)
	input := map[string]string{"status": db.CourierDeliveryPickedUp}
	resp, body := makeAPIRequestOnHost(t, "courier.localhost", http.MethodPatch, path, input, rider, false, nil)
	messagingStatus(t, resp, body, http.StatusForbidden)
	resp, body = makeAPIRequestOnHost(t, "courier.localhost", http.MethodPatch, path, input, buyer, true, nil)
	messagingStatus(t, resp, body, http.StatusForbidden)
	for _, status := range []string{db.CourierDeliveryPickedUp, db.CourierDeliveryOutForDelivery, db.CourierDeliveryDelivered} {
		input["status"] = status
		for range 2 {
			resp, body = makeAPIRequestOnHost(t, "courier.localhost", http.MethodPatch, path, input, rider, true, nil)
			messagingStatus(t, resp, body, http.StatusOK)
		}
	}
	assertView("active", 0)
	assertView("completed", 1)
	resp, body = makeAPIRequestOnHost(t, "courier.localhost", http.MethodGet, "/api/v1/courier/deliveries", nil, rider, false, nil)
	messagingStatus(t, resp, body, http.StatusOK)
	if strings.Contains(string(body), "Sample test address") || !strings.Contains(string(body), `"delivery_address":""`) {
		t.Fatal("completed API history exposed address")
	}
	pageResponse, page := makeRequest(http.MethodGet, "/deliveries", nil, rider, "courier.localhost")
	if pageResponse.StatusCode != http.StatusOK || strings.Contains(page, "Sample test address") || !strings.Contains(page, "Address hidden after delivery completion") {
		t.Fatal("completed HTML history exposed address")
	}
	completedResponse, completedHTML := makeRequest(http.MethodGet, "/deliveries?view=completed", nil, rider, "courier.localhost")
	if completedResponse.StatusCode != http.StatusOK || !strings.Contains(completedHTML, `href="/deliveries?view=completed" aria-current="page"`) {
		t.Fatal("completed view selection missing")
	}
	session, err := testStore.GetSession(t.Context(), rider[0].Value)
	if err != nil || session == nil {
		t.Fatalf("rider session unavailable: %v", err)
	}
	form := url.Values{"csrf_token": {session.CSRFToken}, "status": {db.CourierDeliveryDelivered}, "view": {"completed"}}
	postResponse, _ := makeRequest(http.MethodPost, fmt.Sprintf("/deliveries/%d", jobs[0].ID), form, rider, "courier.localhost")
	if postResponse.StatusCode != http.StatusFound || postResponse.Header.Get("Location") != "/deliveries?view=completed" {
		t.Fatalf("view redirect=%d %s", postResponse.StatusCode, postResponse.Header.Get("Location"))
	}
	var storedAddress string
	if err := testStore.DB.QueryRow(`SELECT delivery_address FROM orders WHERE id=$1`, order).Scan(&storedAddress); err != nil || storedAddress != "Sample test address" {
		t.Fatalf("order address changed: %v", err)
	}
	resp, body = makeAPIRequestOnHost(t, "courier.localhost", http.MethodPatch, path, input, outsider, true, nil)
	messagingStatus(t, resp, body, http.StatusForbidden)
	input["status"] = db.CourierDeliveryPickedUp
	resp, body = makeAPIRequestOnHost(t, "courier.localhost", http.MethodPatch, path, input, rider, true, nil)
	messagingStatus(t, resp, body, http.StatusConflict)
	var count int
	if err := testStore.DB.QueryRow(`SELECT count(*) FROM courier_delivery_events WHERE delivery_id=$1`, jobs[0].ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 4 {
		t.Fatalf("timeline events=%d, want assignment and three handoffs", count)
	}
}

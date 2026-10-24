package main

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/kyambuthia/sokomoko/internal/db"
)

func TestCourierAPIStatusRetriesPreserveAuthorizationAndTimeline(t *testing.T) {
	clearAllTables()
	adminID, _ := messageTestSession(t, "retry-dispatch", db.RoleAdmin, "admin.localhost")
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
	jobs, err := testStore.ListCourierDeliveries(t.Context(), riderID)
	if err != nil || len(jobs) != 1 {
		t.Fatalf("jobs=%+v, %v", jobs, err)
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

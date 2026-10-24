package db

import (
	"testing"
	"time"
)

func TestCourierReassignmentAndFailurePreserveDeliveryBoundary(t *testing.T) {
	s := newTestStore(t)
	buyer := mustCreateUser(t, s, RoleUser)
	admin := mustCreateUser(t, s, RoleAdmin)
	first := mustCreateUser(t, s, RoleCourier)
	replacement := mustCreateUser(t, s, RoleCourier)
	product := mustCreateProduct(t, s, 100, 3)
	if err := s.AddToCart(t.Context(), buyer, product, 1); err != nil {
		t.Fatal(err)
	}
	orderID, err := s.PlaceOrderFromCart(t.Context(), buyer, "demo delivery address")
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range []string{"accepted", "packing", "dispatched"} {
		if err := s.UpdateOrderFulfillment(t.Context(), int(orderID), step, "queued", ""); err != nil {
			t.Fatal(err)
		}
	}
	chatMust(t, s.AssignCourierToOrder(t.Context(), int(orderID), first, admin))
	chatMust(t, s.AssignCourierToOrder(t.Context(), int(orderID), first, admin)) // retry has no extra event
	jobs, err := s.ListCourierDeliveries(t.Context(), first)
	chatMust(t, err)
	if len(jobs) != 1 || jobs[0].DeliveryAddress != "demo delivery address" {
		t.Fatalf("jobs=%+v", jobs)
	}
	deliveryID := jobs[0].ID
	chatWantError(t, s.UpdateCourierDelivery(t.Context(), deliveryID, replacement, CourierDeliveryPickedUp, ""), ErrCourierDeliveryForbidden)
	chatWantError(t, s.UpdateCourierDelivery(t.Context(), deliveryID, first, CourierDeliveryDelivered, ""), ErrInvalidCourierTransition)
	chatWantError(t, s.UpdateCourierDelivery(t.Context(), deliveryID, first, CourierDeliveryFailed, "private customer details"), ErrInvalidCourierTransition)
	chatMust(t, s.UpdateCourierDelivery(t.Context(), deliveryID, first, CourierDeliveryFailed, "Address issue"))
	assertCourierRetriesAreNoOp(t, s, deliveryID, first, CourierDeliveryFailed, "Address issue")
	chatWantError(t, s.UpdateCourierDelivery(t.Context(), deliveryID, first, CourierDeliveryFailed, "Other"), ErrInvalidCourierTransition)
	chatMust(t, s.AssignCourierToOrder(t.Context(), int(orderID), replacement, admin))
	jobs, err = s.ListCourierDeliveries(t.Context(), first)
	chatMust(t, err)
	if len(jobs) != 0 {
		t.Fatal("former courier retained delivery details")
	}
	chatWantError(t, s.UpdateCourierDelivery(t.Context(), deliveryID, first, CourierDeliveryPickedUp, ""), ErrCourierDeliveryForbidden)
	chatMust(t, s.UpdateCourierDelivery(t.Context(), deliveryID, replacement, CourierDeliveryPickedUp, "do not retain free-form personal details"))
	assertCourierRetriesAreNoOp(t, s, deliveryID, replacement, CourierDeliveryPickedUp, "")
	chatWantError(t, s.AssignCourierToOrder(t.Context(), int(orderID), first, admin), ErrDeliveryAlreadyStarted)
	chatMust(t, s.UpdateCourierDelivery(t.Context(), deliveryID, replacement, CourierDeliveryOutForDelivery, ""))
	assertCourierRetriesAreNoOp(t, s, deliveryID, replacement, CourierDeliveryOutForDelivery, "")
	chatMust(t, s.UpdateCourierDelivery(t.Context(), deliveryID, replacement, CourierDeliveryDelivered, ""))
	assertCourierRetriesAreNoOp(t, s, deliveryID, replacement, CourierDeliveryDelivered, "")
	jobs, err = s.ListCourierDeliveries(t.Context(), replacement)
	chatMust(t, err)
	if len(jobs) != 1 || jobs[0].DeliveryAddress != "" {
		t.Fatal("completed courier history retained address")
	}

	chatWantError(t, s.UpdateCourierDelivery(t.Context(), deliveryID, first, CourierDeliveryDelivered, ""), ErrCourierDeliveryForbidden)
	chatWantError(t, s.AssignCourierToOrder(t.Context(), int(orderID), first, admin), ErrDeliveryNotReady)
	var count, privateNotes int
	chatMust(t, s.DB.QueryRow(`SELECT count(*),count(*) FILTER (WHERE note NOT IN ('','Address issue')) FROM courier_delivery_events WHERE delivery_id=$1`, deliveryID).Scan(&count, &privateNotes))
	if count != 6 || privateNotes != 0 {
		t.Fatalf("events=%d, unexpected notes=%d", count, privateNotes)
	}
	if _, err = s.DB.Exec(`UPDATE courier_delivery_events SET note='' WHERE delivery_id=$1`, deliveryID); err == nil {
		t.Fatal("audit events could be edited")
	}
	if _, err = s.DB.Exec(`DELETE FROM courier_delivery_events WHERE delivery_id=$1`, deliveryID); err == nil {
		t.Fatal("audit events could be deleted")
	}
	stock := mustStock(t, s, product)
	if stock.OnHandQuantity != 2 || stock.AllocatedQuantity != 0 {
		t.Fatalf("stock=%+v", stock)
	}
}

func assertCourierRetriesAreNoOp(t *testing.T, s *Store, id, courier int, status, note string) {
	t.Helper()
	var before, after time.Time
	var countBefore, countAfter int
	chatMust(t, s.DB.QueryRow(`SELECT updated_at,(SELECT count(*) FROM courier_delivery_events WHERE delivery_id=$1) FROM courier_deliveries WHERE id=$1`, id).Scan(&before, &countBefore))
	const attempts = 8
	results := make(chan error, attempts)
	start := make(chan struct{})
	for range attempts {
		go func() { <-start; results <- s.UpdateCourierDelivery(t.Context(), id, courier, status, note) }()
	}
	close(start)
	for range attempts {
		chatMust(t, <-results)
	}
	chatMust(t, s.DB.QueryRow(`SELECT updated_at,(SELECT count(*) FROM courier_delivery_events WHERE delivery_id=$1) FROM courier_deliveries WHERE id=$1`, id).Scan(&after, &countAfter))
	if !before.Equal(after) || countBefore != countAfter {
		t.Fatalf("retry mutated delivery: updated=%v -> %v, events=%d -> %d", before, after, countBefore, countAfter)
	}
}

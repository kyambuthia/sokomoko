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
	assertCourierWorkload(t, s, first, 1, 0)
	assertCourierWorkload(t, s, replacement, 0, 0)
	chatWantError(t, s.UpdateCourierDelivery(t.Context(), deliveryID, replacement, CourierDeliveryPickedUp, ""), ErrCourierDeliveryForbidden)
	chatWantError(t, s.UpdateCourierDelivery(t.Context(), deliveryID, first, CourierDeliveryDelivered, ""), ErrInvalidCourierTransition)
	chatWantError(t, s.UpdateCourierDelivery(t.Context(), deliveryID, first, CourierDeliveryFailed, "private customer details"), ErrInvalidCourierTransition)
	chatMust(t, s.UpdateCourierDelivery(t.Context(), deliveryID, first, CourierDeliveryFailed, "Address issue"))
	assertCourierRetriesAreNoOp(t, s, deliveryID, first, CourierDeliveryFailed, "Address issue")
	assertCourierWorkload(t, s, first, 0, 1)
	chatWantError(t, s.UpdateCourierDelivery(t.Context(), deliveryID, first, CourierDeliveryFailed, "Other"), ErrInvalidCourierTransition)
	chatMust(t, s.AssignCourierToOrder(t.Context(), int(orderID), replacement, admin))
	assertCourierWorkload(t, s, first, 0, 0)
	assertCourierWorkload(t, s, replacement, 1, 0)
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
	assertCourierWorkload(t, s, replacement, 0, 0)
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

func assertCourierWorkload(t *testing.T, s *Store, id, active, failed int) {
	t.Helper()
	couriers, err := s.ListCourierUsers(t.Context())
	chatMust(t, err)
	for _, courier := range couriers {
		if courier.ID == id {
			if courier.ActiveDeliveries != active || courier.FailedDeliveries != failed {
				t.Fatalf("courier %d workload=%+v; want active=%d failed=%d", id, courier, active, failed)
			}
			return
		}
	}
	t.Fatalf("courier %d missing", id)
}

func TestCourierWorkloadExcludesTerminalJobsAndInactiveAccounts(t *testing.T) {
	s := newTestStore(t)
	buyer, admin, rider := mustCreateUser(t, s, RoleUser), mustCreateUser(t, s, RoleAdmin), mustCreateUser(t, s, RoleCourier)
	product := mustCreateProduct(t, s, 100, 5)
	for _, terminal := range []struct{ order, partner string }{{"cancelled", "cancelled"}, {"delivered", "completed"}} {
		chatMust(t, s.AddToCart(t.Context(), buyer, product, 1))
		id, err := s.PlaceOrderFromCart(t.Context(), buyer, "fixture destination")
		chatMust(t, err)
		for _, step := range []string{"accepted", "packing", "dispatched"} {
			chatMust(t, s.UpdateOrderFulfillment(t.Context(), int(id), step, "queued", ""))
		}
		chatMust(t, s.AssignCourierToOrder(t.Context(), int(id), rider, admin))
		assertCourierWorkload(t, s, rider, 1, 0)
		chatMust(t, s.UpdateOrderByAdmin(t.Context(), int(id), terminal.order, terminal.partner, "queued", ""))
		assertCourierWorkload(t, s, rider, 0, 0)
	}
	_, err := s.DB.Exec(`UPDATE users SET deleted_at=now() WHERE id=$1`, rider)
	chatMust(t, err)
	couriers, err := s.ListCourierUsers(t.Context())
	chatMust(t, err)
	for _, courier := range couriers {
		if courier.ID == rider || courier.ID == admin {
			t.Fatal("inactive or non-courier account exposed in dispatch options")
		}
	}
}

func TestCourierAdministrativeCompletionClosesAccessAndHandoffs(t *testing.T) {
	for _, state := range []struct{ name, order, delivery string }{
		{"order", "delivered", "shipped"}, {"delivery", "shipped", "delivered"}, {"both", "delivered", "delivered"},
	} {
		t.Run(state.name, func(t *testing.T) {
			s := newTestStore(t)
			buyer, admin, rider := mustCreateUser(t, s, RoleUser), mustCreateUser(t, s, RoleAdmin), mustCreateUser(t, s, RoleCourier)
			product := mustCreateProduct(t, s, 100, 2)
			chatMust(t, s.AddToCart(t.Context(), buyer, product, 1))
			order, err := s.PlaceOrderFromCart(t.Context(), buyer, "private fixture address")
			chatMust(t, err)
			for _, step := range []string{"accepted", "packing", "dispatched"} {
				chatMust(t, s.UpdateOrderFulfillment(t.Context(), int(order), step, "queued", ""))
			}
			chatMust(t, s.AssignCourierToOrder(t.Context(), int(order), rider, admin))
			a, b := newEncryptedTestDevice(t, s, buyer, 900), newEncryptedTestDevice(t, s, rider, 901)
			encryptedPublish(t, s, b, 902)
			target := ChatTarget{DeliveryOrderID: int(order)}
			id := encryptedStart(t, s, a, target)
			session := encryptedNewSession(t, s, a, id, 903)
			first, err := a.call(t, s, "send", encryptedSendInput(id, session, 0))
			chatMust(t, err)
			chatMust(t, s.UpdateOrderByAdmin(t.Context(), int(order), state.order, "completed", state.delivery, "Completed by operations."))
			jobs, err := s.ListCourierDeliveries(t.Context(), rider)
			chatMust(t, err)
			if len(jobs) != 1 || jobs[0].Status != CourierDeliveryDelivered || jobs[0].DeliveryAddress != "" {
				t.Fatalf("completed projection=%+v", jobs)
			}
			assertCourierWorkload(t, s, rider, 0, 0)
			chatWantError(t, s.UpdateCourierDelivery(t.Context(), jobs[0].ID, rider, CourierDeliveryPickedUp, ""), ErrInvalidCourierTransition)
			for _, actor := range []int{buyer, rider} {
				_, err = s.ChatContacts(t.Context(), actor, target)
				chatWantError(t, err, ErrChatNotFound)
				thread, err := s.ChatThread(t.Context(), actor, id, 0, 0)
				chatMust(t, err)
				if thread.Conversation.CanSend || !thread.Conversation.Closed {
					t.Fatal("completed delivery conversation remained open")
				}
			}
			_, err = a.call(t, s, "send", encryptedSendInput(id, session, first.ID))
			chatWantError(t, err, ErrChatClosed)
			_, err = a.call(t, s, "start", map[string]any{"target": target})
			chatWantError(t, err, ErrChatNotFound)
			var storedStatus string
			var events int
			chatMust(t, s.DB.QueryRow(`SELECT status FROM courier_deliveries WHERE id=$1`, jobs[0].ID).Scan(&storedStatus))
			chatMust(t, s.DB.QueryRow(`SELECT count(*) FROM courier_delivery_events WHERE delivery_id=$1`, jobs[0].ID).Scan(&events))
			if storedStatus != CourierDeliveryAssigned || events != 1 {
				t.Fatal("administrative completion fabricated a rider handoff")
			}
		})
	}
}

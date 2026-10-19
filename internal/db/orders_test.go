package db

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kyambuthia/sokomoko/internal/money"
)

func prepareCheckout(t *testing.T, s *Store, userID int, token string, ttl time.Duration) *Checkout {
	t.Helper()
	if err := s.ReserveCartForCheckout(t.Context(), userID, token, timeNow().Add(ttl)); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	items, subtotal, err := s.GetCartItemsForCheckout(t.Context(), userID, token)
	if err != nil {
		t.Fatalf("cart items: %v", err)
	}
	lines := make([]CheckoutLineInput, 0, len(items))
	for _, item := range items {
		lines = append(lines, CheckoutLineInput{ProductID: item.ProductID, ProductName: item.ProductName, Quantity: item.Quantity, UnitPrice: item.UnitPrice})
	}
	shipping := money.Cents(650)
	tax := subtotal.MulBasisPoints(800)
	checkout, err := s.UpsertCheckout(t.Context(), userID, CheckoutInput{
		Token:          token,
		SubtotalAmount: subtotal,
		ShippingFee:    shipping,
		TaxAmount:      tax,
		TotalAmount:    subtotal + shipping + tax,
		ExpiresAt:      timeNow().Add(ttl),
		Lines:          lines,
	})
	if err != nil {
		t.Fatalf("upsert checkout: %v", err)
	}
	return checkout
}

var codPayment = PaymentRecordInput{Method: "cash_on_delivery", Provider: "manual", Status: PaymentStatusPending}

func TestCartAndDirectOrderFlow(t *testing.T) {
	s := newTestStore(t)
	userID := mustCreateUser(t, s, RoleUser)
	productID := mustCreateProduct(t, s, 1999, 5)

	if err := s.AddToCart(t.Context(), userID, productID, 2); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := s.AddToCart(t.Context(), userID, productID, 1); err != nil {
		t.Fatalf("add again: %v", err)
	}
	items, subtotal, err := s.GetCartItems(t.Context(), userID)
	if err != nil || len(items) != 1 || items[0].Quantity != 3 || subtotal != 5997 {
		t.Fatalf("cart = %+v subtotal=%d err=%v", items, subtotal, err)
	}
	if count, _ := s.CartItemCount(t.Context(), userID); count != 3 {
		t.Fatalf("cart count = %d", count)
	}

	if _, err := s.PlaceOrderFromCart(t.Context(), userID, "  "); !errors.Is(err, ErrDeliveryAddressRequired) {
		t.Fatalf("blank address err = %v", err)
	}
	orderID, err := s.PlaceOrderFromCart(t.Context(), userID, "1 Test Street")
	if err != nil {
		t.Fatalf("place order: %v", err)
	}
	if _, err := s.PlaceOrderFromCart(t.Context(), userID, "1 Test Street"); !errors.Is(err, ErrCartEmpty) {
		t.Fatalf("second order err = %v, want ErrCartEmpty", err)
	}

	orders, _ := s.ListOrdersByUser(t.Context(), userID)
	if len(orders) != 1 || orders[0].ID != int(orderID) || orders[0].TotalAmount != 5997 || len(orders[0].Items) != 1 {
		t.Fatalf("orders = %+v", orders)
	}
	if orders[0].Items[0].LineTotal != 5997 {
		t.Fatalf("line total = %d", orders[0].Items[0].LineTotal)
	}
	if stock := mustStock(t, s, productID); stock.AllocatedQuantity != 3 || stock.AvailableQuantity != 2 {
		t.Fatalf("stock after order = %+v", stock)
	}
}

func TestAddToCart_RemovesDeletedProductsFromCart(t *testing.T) {
	s := newTestStore(t)
	userID := mustCreateUser(t, s, RoleUser)
	productID := mustCreateProduct(t, s, 100, 5)
	_ = s.AddToCart(t.Context(), userID, productID, 1)
	if err := s.DeleteProduct(t.Context(), productID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if items, _, _ := s.GetCartItems(t.Context(), userID); len(items) != 0 {
		t.Fatalf("deleted product still in cart")
	}
}

func TestReserveCartForCheckout_RefreshesHolds(t *testing.T) {
	s := newTestStore(t)
	userID := mustCreateUser(t, s, RoleUser)
	productID := mustCreateProduct(t, s, 500, 6)

	_ = s.AddToCart(t.Context(), userID, productID, 2)
	if err := s.ReserveCartForCheckout(t.Context(), userID, "first", timeNow().Add(time.Minute)); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if got := mustAvailable(t, s, productID); got != 4 {
		t.Fatalf("available after reserve = %d, want 4", got)
	}
	// Reserving again under a new key replaces the earlier hold.
	if err := s.ReserveCartForCheckout(t.Context(), userID, "second", timeNow().Add(time.Minute)); err != nil {
		t.Fatalf("re-reserve: %v", err)
	}
	if got := mustAvailable(t, s, productID); got != 4 {
		t.Fatalf("available after re-reserve = %d, want 4", got)
	}
	if active, _ := s.ListActiveStockReservationsByKey(t.Context(), userID, "first"); len(active) != 0 {
		t.Fatalf("old reservation still active")
	}
	items, _, _ := s.GetCartItemsForCheckout(t.Context(), userID, "second")
	if len(items) != 1 || items[0].StockQuantity != 6 {
		t.Fatalf("own hold should count as available: %+v", items)
	}

	other := mustCreateUser(t, s, RoleUser)
	_ = s.AddToCart(t.Context(), other, productID, 5)
	if err := s.ReserveCartForCheckout(t.Context(), other, "other", timeNow().Add(time.Minute)); !errors.Is(err, ErrInsufficientStock) {
		t.Fatalf("over-reserve err = %v, want ErrInsufficientStock", err)
	}
}

func TestReleaseExpiredReservations(t *testing.T) {
	s := newTestStore(t)
	userID := mustCreateUser(t, s, RoleUser)
	productID := mustCreateProduct(t, s, 500, 3)
	_ = s.AddToCart(t.Context(), userID, productID, 3)
	checkout := prepareCheckout(t, s, userID, "soon", -time.Second)

	released, err := s.ReleaseExpiredReservations(t.Context())
	if err != nil || released != 1 {
		t.Fatalf("released = %d, %v", released, err)
	}
	if got := mustAvailable(t, s, productID); got != 3 {
		t.Fatalf("available after expiry = %d, want 3", got)
	}
	reloaded, _ := s.GetCheckoutByToken(t.Context(), userID, checkout.Token)
	if reloaded.Status != CheckoutStatusExpired {
		t.Fatalf("checkout status = %s, want expired", reloaded.Status)
	}
	if _, err := s.PlaceOrderFromCheckoutWithPayment(t.Context(), userID, checkout.Token, "addr", "", codPayment); !errors.Is(err, ErrCheckoutExpired) {
		t.Fatalf("place expired err = %v", err)
	}
}

func TestPlaceOrderFromCheckout_ConvertsHoldsAndIsIdempotent(t *testing.T) {
	s := newTestStore(t)
	userID := mustCreateUser(t, s, RoleUser)
	productID := mustCreateProduct(t, s, 2500, 4)
	_ = s.AddToCart(t.Context(), userID, productID, 2)
	checkout := prepareCheckout(t, s, userID, "tok-1", 15*time.Minute)

	if checkout.SubtotalAmount != 5000 || checkout.TaxAmount != 400 || checkout.TotalAmount != 6050 || len(checkout.Lines) != 1 {
		t.Fatalf("checkout = %+v", checkout)
	}
	if _, err := s.UpdateCheckoutDraft(t.Context(), userID, "tok-1", "Saved address", "card_placeholder"); err != nil {
		t.Fatalf("draft: %v", err)
	}

	placement, err := s.PlaceOrderFromCheckoutWithPayment(t.Context(), userID, "tok-1", "9 Market Rd", "", codPayment)
	if err != nil {
		t.Fatalf("place: %v", err)
	}
	if placement.Reused || placement.TotalAmount != 6050 {
		t.Fatalf("placement = %+v", placement)
	}

	again, err := s.PlaceOrderFromCheckoutWithPayment(t.Context(), userID, "tok-1", "9 Market Rd", "", codPayment)
	if err != nil || !again.Reused || again.OrderID != placement.OrderID {
		t.Fatalf("replay = %+v, %v", again, err)
	}

	stock := mustStock(t, s, productID)
	if stock.ReservedQuantity != 0 || stock.AllocatedQuantity != 2 || stock.AvailableQuantity != 2 {
		t.Fatalf("stock = %+v", stock)
	}
	orders, _ := s.ListOrdersByUser(t.Context(), userID)
	if len(orders) != 1 || orders[0].ShippingFee != 650 || orders[0].TaxAmount != 400 || orders[0].Subtotal != 5000 {
		t.Fatalf("orders = %+v", orders)
	}
	payments, _ := s.ListPaymentsByOrderID(t.Context(), int(placement.OrderID))
	if len(payments) != 1 || payments[0].Amount != 6050 {
		t.Fatalf("payments = %+v", payments)
	}
	if items, _, _ := s.GetCartItems(t.Context(), userID); len(items) != 0 {
		t.Fatalf("cart not cleared")
	}
	completed, _ := s.GetCheckoutByToken(t.Context(), userID, "tok-1")
	if completed.Status != CheckoutStatusCompleted || !completed.OrderID.Valid {
		t.Fatalf("checkout = %+v", completed)
	}

	// A completed checkout token cannot be reopened.
	if _, err := s.UpsertCheckout(t.Context(), userID, CheckoutInput{Token: "tok-1", Lines: []CheckoutLineInput{{ProductID: productID, ProductName: "x", Quantity: 1}}}); !errors.Is(err, ErrCheckoutNotFound) {
		t.Fatalf("reopen completed err = %v", err)
	}
}

func TestCartChangeInvalidatesOpenCheckout(t *testing.T) {
	s := newTestStore(t)
	userID := mustCreateUser(t, s, RoleUser)
	a := mustCreateProduct(t, s, 100, 5)
	b := mustCreateProduct(t, s, 100, 5)
	_ = s.AddToCart(t.Context(), userID, a, 1)
	checkout := prepareCheckout(t, s, userID, "stale", 15*time.Minute)

	if err := s.AddToCart(t.Context(), userID, b, 1); err != nil {
		t.Fatalf("add: %v", err)
	}
	reloaded, _ := s.GetCheckoutByToken(t.Context(), userID, checkout.Token)
	if reloaded.Status != CheckoutStatusCancelled {
		t.Fatalf("checkout status after cart change = %s, want cancelled", reloaded.Status)
	}
	if got := mustAvailable(t, s, a); got != 5 {
		t.Fatalf("hold not released: available = %d", got)
	}
	if _, err := s.PlaceOrderFromCheckoutWithPayment(t.Context(), userID, checkout.Token, "addr", "", codPayment); !errors.Is(err, ErrCheckoutNotFound) {
		t.Fatalf("stale checkout err = %v", err)
	}
}

func TestFulfillmentTransitionsMoveInventory(t *testing.T) {
	s := newTestStore(t)
	userID := mustCreateUser(t, s, RoleUser)
	productID := mustCreateProduct(t, s, 100, 10)

	place := func(qty int) int {
		_ = s.AddToCart(t.Context(), userID, productID, qty)
		id, err := s.PlaceOrderFromCart(t.Context(), userID, "addr")
		if err != nil {
			t.Fatalf("place: %v", err)
		}
		return int(id)
	}

	shipped := place(2)
	cancelled := place(3)
	if stock := mustStock(t, s, productID); stock.AllocatedQuantity != 5 {
		t.Fatalf("allocated = %d, want 5", stock.AllocatedQuantity)
	}

	if err := s.UpdateOrderFulfillment(t.Context(), shipped, "dispatched", "shipped", ""); !errors.Is(err, ErrInvalidPartnerTransition) {
		t.Fatalf("skip-ahead err = %v", err)
	}
	for _, step := range []string{"accepted", "packing", "dispatched"} {
		if err := s.UpdateOrderFulfillment(t.Context(), shipped, step, "processing", step); err != nil {
			t.Fatalf("%s: %v", step, err)
		}
	}
	if err := s.UpdateOrderFulfillment(t.Context(), shipped, "completed", "queued", ""); !errors.Is(err, ErrInvalidDeliveryTransition) {
		t.Fatalf("delivery regression err = %v", err)
	}
	if err := s.UpdateOrderFulfillment(t.Context(), shipped, "completed", "delivered", "done"); err != nil {
		t.Fatalf("complete: %v", err)
	}

	if err := s.UpdateOrderFulfillment(t.Context(), cancelled, "cancelled", "queued", "out of stock"); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	stock := mustStock(t, s, productID)
	if stock.OnHandQuantity != 8 || stock.AllocatedQuantity != 0 || stock.AvailableQuantity != 8 {
		t.Fatalf("stock = %+v, want on_hand 8, allocated 0", stock)
	}

	if err := s.UpdateOrderByAdmin(t.Context(), cancelled, OrderStatusPending, "new", "queued", ""); !errors.Is(err, ErrInvalidOrderState) {
		t.Fatalf("reopen cancelled err = %v", err)
	}
	if err := s.UpdateOrderByAdmin(t.Context(), cancelled, "bogus", "new", "queued", ""); !errors.Is(err, ErrInvalidOrderState) {
		t.Fatalf("invalid status err = %v", err)
	}
	if err := s.UpdateOrderFulfillment(t.Context(), 999999, "accepted", "queued", ""); !errors.Is(err, ErrOrderNotFound) {
		t.Fatalf("missing order err = %v", err)
	}
	if err := s.UpdateOrderByAdmin(t.Context(), 999999, OrderStatusPending, "new", "queued", ""); !errors.Is(err, ErrOrderNotFound) {
		t.Fatalf("missing order err = %v", err)
	}

	summary, err := s.GetPartnerOrderSummary(t.Context())
	if err != nil || summary.CompletedCount != 1 {
		t.Fatalf("summary = %+v, %v", summary, err)
	}
	counts, _ := s.GetOrderStatusCounts(t.Context())
	if counts[OrderStatusDelivered] != 1 || counts[OrderStatusCancelled] != 1 {
		t.Fatalf("counts = %+v", counts)
	}
	if revenue, _ := s.SumOrderRevenue(t.Context()); revenue != 200 {
		t.Fatalf("revenue = %d, want 200", revenue)
	}
}

func TestConcurrentCheckoutNeverOversells(t *testing.T) {
	s := newTestStore(t)
	productID := mustCreateProduct(t, s, 100, 3)

	const buyers = 8
	userIDs := make([]int, buyers)
	for i := range userIDs {
		userIDs[i] = mustCreateUser(t, s, RoleUser)
		if err := s.AddToCart(t.Context(), userIDs[i], productID, 1); err != nil {
			t.Fatalf("add: %v", err)
		}
	}

	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		successes int
	)
	for _, userID := range userIDs {
		wg.Add(1)
		go func(userID int) {
			defer wg.Done()
			_, err := s.PlaceOrderFromCart(t.Context(), userID, "Concurrent Street")
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				successes++
			case errors.Is(err, ErrInsufficientStock):
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}(userID)
	}
	wg.Wait()

	if successes != 3 {
		t.Fatalf("successful orders = %d, want 3", successes)
	}
	if stock := mustStock(t, s, productID); stock.AvailableQuantity != 0 || stock.AllocatedQuantity != 3 {
		t.Fatalf("stock = %+v", stock)
	}
}

func TestAuditLogs(t *testing.T) {
	s := newTestStore(t)
	actor := mustCreateUser(t, s, RoleAdmin)
	if err := s.CreateAuditLog(t.Context(), actor, "order.update", "order", 5, "status=shipped"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := s.CreateAuditLog(t.Context(), 0, "", "order", 0, ""); err == nil {
		t.Fatalf("expected validation error")
	}
	logs, err := s.ListAuditLogs(t.Context(), 10)
	if err != nil || len(logs) != 1 || logs[0].ActorName == "" || !logs[0].TargetID.Valid {
		t.Fatalf("logs = %+v, %v", logs, err)
	}
}

func TestSeed_IsIdempotent(t *testing.T) {
	s := newTestStore(t)
	for i := 0; i < 2; i++ {
		s.SeedPartners(t.Context())
		s.SeedInitialCatalog(t.Context())
	}
	count, _ := s.CountProducts(t.Context())
	if count != len(demoCatalog) {
		t.Fatalf("products = %d, want %d", count, len(demoCatalog))
	}
	partners, _ := s.ListPartners(t.Context())
	if len(partners) != len(demoPartners) {
		t.Fatalf("partners = %d", len(partners))
	}
	product, _ := s.GetProductBySlug(t.Context(), "flagship-smartphone")
	if product == nil || product.PartnerName != "Urban Goods" || product.Price != 99900 {
		t.Fatalf("seeded product = %+v", product)
	}
}

func TestPartnerOrderOwnershipRequiresEveryLine(t *testing.T) {
	s := newTestStore(t)
	partnerA, err := s.UpsertPartner(t.Context(), Partner{Name: "Owner A", Slug: "owner_a_" + nextSuffix()})
	if err != nil {
		t.Fatal(err)
	}
	partnerB, err := s.UpsertPartner(t.Context(), Partner{Name: "Owner B", Slug: "owner_b_" + nextSuffix()})
	if err != nil {
		t.Fatal(err)
	}
	productFor := func(partnerID int64) int {
		id := mustCreateProduct(t, s, 1000, 10)
		if partnerID > 0 {
			if _, err := s.DB.Exec("UPDATE products SET partner_id = $1 WHERE id = $2", partnerID, id); err != nil {
				t.Fatal(err)
			}
		}
		return id
	}
	placeOrder := func(products ...int) int {
		userID := mustCreateUser(t, s, RoleUser)
		for _, productID := range products {
			if err := s.AddToCart(t.Context(), userID, productID, 1); err != nil {
				t.Fatal(err)
			}
		}
		orderID, err := s.PlaceOrderFromCart(t.Context(), userID, "1 Owner Lane")
		if err != nil {
			t.Fatal(err)
		}
		return int(orderID)
	}
	owned := placeOrder(productFor(partnerA), productFor(partnerA))
	mixed := placeOrder(productFor(partnerA), productFor(partnerB))
	unassigned := placeOrder(productFor(partnerA), productFor(0))

	for _, tc := range []struct {
		name      string
		partnerID int
		orderID   int
		want      bool
	}{
		{"owned", int(partnerA), owned, true},
		{"other partner", int(partnerB), owned, false},
		{"mixed", int(partnerA), mixed, false},
		{"unassigned line", int(partnerA), unassigned, false},
		{"no membership", 0, owned, false},
	} {
		got, err := s.PartnerOwnsOrder(t.Context(), tc.partnerID, tc.orderID)
		if err != nil || got != tc.want {
			t.Errorf("%s: PartnerOwnsOrder = %v, %v; want %v", tc.name, got, err, tc.want)
		}
	}

	orders, err := s.ListOrdersForPartner(t.Context(), int(partnerA))
	if err != nil || len(orders) != 1 || orders[0].ID != owned {
		t.Fatalf("ListOrdersForPartner = %+v, %v; want only order %d", orders, err, owned)
	}

	if err := s.UpdateOrderFulfillment(t.Context(), owned, "cancelled", "queued", "cancelled"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.PartnerOwnsOrder(t.Context(), int(partnerA), owned); got {
		t.Fatal("cancelled orders must not be partner fulfillment work")
	}
}

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
	if err := s.ReserveCartForCheckout(userID, token, timeNow().Add(ttl)); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	items, subtotal, err := s.GetCartItemsForCheckout(userID, token)
	if err != nil {
		t.Fatalf("cart items: %v", err)
	}
	lines := make([]CheckoutLineInput, 0, len(items))
	for _, item := range items {
		lines = append(lines, CheckoutLineInput{ProductID: item.ProductID, ProductName: item.ProductName, Quantity: item.Quantity, UnitPrice: item.UnitPrice})
	}
	shipping := money.Cents(650)
	tax := subtotal.MulBasisPoints(800)
	checkout, err := s.UpsertCheckout(userID, CheckoutInput{
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

	if err := s.AddToCart(userID, productID, 2); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := s.AddToCart(userID, productID, 1); err != nil {
		t.Fatalf("add again: %v", err)
	}
	items, subtotal, err := s.GetCartItems(userID)
	if err != nil || len(items) != 1 || items[0].Quantity != 3 || subtotal != 5997 {
		t.Fatalf("cart = %+v subtotal=%d err=%v", items, subtotal, err)
	}
	if count, _ := s.CartItemCount(userID); count != 3 {
		t.Fatalf("cart count = %d", count)
	}

	if _, err := s.PlaceOrderFromCart(userID, "  "); !errors.Is(err, ErrDeliveryAddressRequired) {
		t.Fatalf("blank address err = %v", err)
	}
	orderID, err := s.PlaceOrderFromCart(userID, "1 Test Street")
	if err != nil {
		t.Fatalf("place order: %v", err)
	}
	if _, err := s.PlaceOrderFromCart(userID, "1 Test Street"); !errors.Is(err, ErrCartEmpty) {
		t.Fatalf("second order err = %v, want ErrCartEmpty", err)
	}

	orders, _ := s.ListOrdersByUser(userID)
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
	_ = s.AddToCart(userID, productID, 1)
	if err := s.DeleteProduct(productID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if items, _, _ := s.GetCartItems(userID); len(items) != 0 {
		t.Fatalf("deleted product still in cart")
	}
}

func TestReserveCartForCheckout_RefreshesHolds(t *testing.T) {
	s := newTestStore(t)
	userID := mustCreateUser(t, s, RoleUser)
	productID := mustCreateProduct(t, s, 500, 6)

	_ = s.AddToCart(userID, productID, 2)
	if err := s.ReserveCartForCheckout(userID, "first", timeNow().Add(time.Minute)); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if got := mustAvailable(t, s, productID); got != 4 {
		t.Fatalf("available after reserve = %d, want 4", got)
	}
	// Reserving again under a new key replaces the earlier hold.
	if err := s.ReserveCartForCheckout(userID, "second", timeNow().Add(time.Minute)); err != nil {
		t.Fatalf("re-reserve: %v", err)
	}
	if got := mustAvailable(t, s, productID); got != 4 {
		t.Fatalf("available after re-reserve = %d, want 4", got)
	}
	if active, _ := s.ListActiveStockReservationsByKey(userID, "first"); len(active) != 0 {
		t.Fatalf("old reservation still active")
	}
	items, _, _ := s.GetCartItemsForCheckout(userID, "second")
	if len(items) != 1 || items[0].StockQuantity != 6 {
		t.Fatalf("own hold should count as available: %+v", items)
	}

	other := mustCreateUser(t, s, RoleUser)
	_ = s.AddToCart(other, productID, 5)
	if err := s.ReserveCartForCheckout(other, "other", timeNow().Add(time.Minute)); !errors.Is(err, ErrInsufficientStock) {
		t.Fatalf("over-reserve err = %v, want ErrInsufficientStock", err)
	}
}

func TestReleaseExpiredReservations(t *testing.T) {
	s := newTestStore(t)
	userID := mustCreateUser(t, s, RoleUser)
	productID := mustCreateProduct(t, s, 500, 3)
	_ = s.AddToCart(userID, productID, 3)
	checkout := prepareCheckout(t, s, userID, "soon", -time.Second)

	released, err := s.ReleaseExpiredReservations()
	if err != nil || released != 1 {
		t.Fatalf("released = %d, %v", released, err)
	}
	if got := mustAvailable(t, s, productID); got != 3 {
		t.Fatalf("available after expiry = %d, want 3", got)
	}
	reloaded, _ := s.GetCheckoutByToken(userID, checkout.Token)
	if reloaded.Status != CheckoutStatusExpired {
		t.Fatalf("checkout status = %s, want expired", reloaded.Status)
	}
	if _, err := s.PlaceOrderFromCheckoutWithPayment(userID, checkout.Token, "addr", "", codPayment); !errors.Is(err, ErrCheckoutExpired) {
		t.Fatalf("place expired err = %v", err)
	}
}

func TestPlaceOrderFromCheckout_ConvertsHoldsAndIsIdempotent(t *testing.T) {
	s := newTestStore(t)
	userID := mustCreateUser(t, s, RoleUser)
	productID := mustCreateProduct(t, s, 2500, 4)
	_ = s.AddToCart(userID, productID, 2)
	checkout := prepareCheckout(t, s, userID, "tok-1", 15*time.Minute)

	if checkout.SubtotalAmount != 5000 || checkout.TaxAmount != 400 || checkout.TotalAmount != 6050 || len(checkout.Lines) != 1 {
		t.Fatalf("checkout = %+v", checkout)
	}
	if _, err := s.UpdateCheckoutDraft(userID, "tok-1", "Saved address", "card_placeholder"); err != nil {
		t.Fatalf("draft: %v", err)
	}

	placement, err := s.PlaceOrderFromCheckoutWithPayment(userID, "tok-1", "9 Market Rd", "", codPayment)
	if err != nil {
		t.Fatalf("place: %v", err)
	}
	if placement.Reused || placement.TotalAmount != 6050 {
		t.Fatalf("placement = %+v", placement)
	}

	again, err := s.PlaceOrderFromCheckoutWithPayment(userID, "tok-1", "9 Market Rd", "", codPayment)
	if err != nil || !again.Reused || again.OrderID != placement.OrderID {
		t.Fatalf("replay = %+v, %v", again, err)
	}

	stock := mustStock(t, s, productID)
	if stock.ReservedQuantity != 0 || stock.AllocatedQuantity != 2 || stock.AvailableQuantity != 2 {
		t.Fatalf("stock = %+v", stock)
	}
	orders, _ := s.ListOrdersByUser(userID)
	if len(orders) != 1 || orders[0].ShippingFee != 650 || orders[0].TaxAmount != 400 || orders[0].Subtotal != 5000 {
		t.Fatalf("orders = %+v", orders)
	}
	payments, _ := s.ListPaymentsByOrderID(int(placement.OrderID))
	if len(payments) != 1 || payments[0].Amount != 6050 {
		t.Fatalf("payments = %+v", payments)
	}
	if items, _, _ := s.GetCartItems(userID); len(items) != 0 {
		t.Fatalf("cart not cleared")
	}
	completed, _ := s.GetCheckoutByToken(userID, "tok-1")
	if completed.Status != CheckoutStatusCompleted || !completed.OrderID.Valid {
		t.Fatalf("checkout = %+v", completed)
	}

	// A completed checkout token cannot be reopened.
	if _, err := s.UpsertCheckout(userID, CheckoutInput{Token: "tok-1", Lines: []CheckoutLineInput{{ProductID: productID, ProductName: "x", Quantity: 1}}}); !errors.Is(err, ErrCheckoutNotFound) {
		t.Fatalf("reopen completed err = %v", err)
	}
}

func TestCartChangeInvalidatesOpenCheckout(t *testing.T) {
	s := newTestStore(t)
	userID := mustCreateUser(t, s, RoleUser)
	a := mustCreateProduct(t, s, 100, 5)
	b := mustCreateProduct(t, s, 100, 5)
	_ = s.AddToCart(userID, a, 1)
	checkout := prepareCheckout(t, s, userID, "stale", 15*time.Minute)

	if err := s.AddToCart(userID, b, 1); err != nil {
		t.Fatalf("add: %v", err)
	}
	reloaded, _ := s.GetCheckoutByToken(userID, checkout.Token)
	if reloaded.Status != CheckoutStatusCancelled {
		t.Fatalf("checkout status after cart change = %s, want cancelled", reloaded.Status)
	}
	if got := mustAvailable(t, s, a); got != 5 {
		t.Fatalf("hold not released: available = %d", got)
	}
	if _, err := s.PlaceOrderFromCheckoutWithPayment(userID, checkout.Token, "addr", "", codPayment); !errors.Is(err, ErrCheckoutNotFound) {
		t.Fatalf("stale checkout err = %v", err)
	}
}

func TestFulfillmentTransitionsMoveInventory(t *testing.T) {
	s := newTestStore(t)
	userID := mustCreateUser(t, s, RoleUser)
	productID := mustCreateProduct(t, s, 100, 10)

	place := func(qty int) int {
		_ = s.AddToCart(userID, productID, qty)
		id, err := s.PlaceOrderFromCart(userID, "addr")
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

	if err := s.UpdateOrderFulfillment(shipped, "dispatched", "shipped", ""); !errors.Is(err, ErrInvalidPartnerTransition) {
		t.Fatalf("skip-ahead err = %v", err)
	}
	for _, step := range []string{"accepted", "packing", "dispatched"} {
		if err := s.UpdateOrderFulfillment(shipped, step, "processing", step); err != nil {
			t.Fatalf("%s: %v", step, err)
		}
	}
	if err := s.UpdateOrderFulfillment(shipped, "completed", "queued", ""); !errors.Is(err, ErrInvalidDeliveryTransition) {
		t.Fatalf("delivery regression err = %v", err)
	}
	if err := s.UpdateOrderFulfillment(shipped, "completed", "delivered", "done"); err != nil {
		t.Fatalf("complete: %v", err)
	}

	if err := s.UpdateOrderFulfillment(cancelled, "cancelled", "queued", "out of stock"); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	stock := mustStock(t, s, productID)
	if stock.OnHandQuantity != 8 || stock.AllocatedQuantity != 0 || stock.AvailableQuantity != 8 {
		t.Fatalf("stock = %+v, want on_hand 8, allocated 0", stock)
	}

	if err := s.UpdateOrderByAdmin(cancelled, OrderStatusPending, "new", "queued", ""); !errors.Is(err, ErrInvalidOrderState) {
		t.Fatalf("reopen cancelled err = %v", err)
	}
	if err := s.UpdateOrderByAdmin(cancelled, "bogus", "new", "queued", ""); !errors.Is(err, ErrInvalidOrderState) {
		t.Fatalf("invalid status err = %v", err)
	}
	if err := s.UpdateOrderFulfillment(999999, "accepted", "queued", ""); !errors.Is(err, ErrOrderNotFound) {
		t.Fatalf("missing order err = %v", err)
	}
	if err := s.UpdateOrderByAdmin(999999, OrderStatusPending, "new", "queued", ""); !errors.Is(err, ErrOrderNotFound) {
		t.Fatalf("missing order err = %v", err)
	}

	summary, err := s.GetPartnerOrderSummary()
	if err != nil || summary.CompletedCount != 1 {
		t.Fatalf("summary = %+v, %v", summary, err)
	}
	counts, _ := s.GetOrderStatusCounts()
	if counts[OrderStatusDelivered] != 1 || counts[OrderStatusCancelled] != 1 {
		t.Fatalf("counts = %+v", counts)
	}
	if revenue, _ := s.SumOrderRevenue(); revenue != 200 {
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
		if err := s.AddToCart(userIDs[i], productID, 1); err != nil {
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
			_, err := s.PlaceOrderFromCart(userID, "Concurrent Street")
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
	if err := s.CreateAuditLog(actor, "order.update", "order", 5, "status=shipped"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := s.CreateAuditLog(0, "", "order", 0, ""); err == nil {
		t.Fatalf("expected validation error")
	}
	logs, err := s.ListAuditLogs(10)
	if err != nil || len(logs) != 1 || logs[0].ActorName == "" || !logs[0].TargetID.Valid {
		t.Fatalf("logs = %+v, %v", logs, err)
	}
}

func TestSeed_IsIdempotent(t *testing.T) {
	s := newTestStore(t)
	for i := 0; i < 2; i++ {
		s.SeedPartners()
		s.SeedInitialCatalog()
	}
	count, _ := s.CountProducts()
	if count != len(demoCatalog) {
		t.Fatalf("products = %d, want %d", count, len(demoCatalog))
	}
	partners, _ := s.ListPartners()
	if len(partners) != len(demoPartners) {
		t.Fatalf("partners = %d", len(partners))
	}
	product, _ := s.GetProductBySlug("flagship-smartphone")
	if product == nil || product.PartnerName != "Urban Goods" || product.Price != 99900 {
		t.Fatalf("seeded product = %+v", product)
	}
}

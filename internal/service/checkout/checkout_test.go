package checkout

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/db/dbtest"
	paymentsvc "github.com/kyambuthia/sokomoko/internal/service/payment"
)

func newTestService(t *testing.T) (*Service, *db.Store, func()) {
	t.Helper()

	store, err := db.OpenStore(dbtest.DSN(t))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := store.ApplySchema(); err != nil {
		t.Fatalf("apply schema: %v", err)
	}

	cleanup := func() { _ = store.Close() }

	return New(store, paymentsvc.New()), store, cleanup
}

func createUserAndProduct(t *testing.T, store *db.Store, stock int) (int, int) {
	t.Helper()

	userID64, err := store.CreateUser(db.User{
		Username:     "u_test",
		Email:        "u_test@example.com",
		PasswordHash: "hash",
		Role:         "user",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	catID, err := store.CreateCategory(db.Category{
		Name: "Cat A",
		Slug: "cat-a",
	})
	if err != nil {
		t.Fatalf("create category: %v", err)
	}

	productID64, err := store.CreateProduct(db.Product{
		Name:          "Product A",
		Slug:          "product-a",
		Description:   "desc",
		Price:         1000,
		StockQuantity: stock,
		CategoryID:    sql.NullInt64{Int64: catID, Valid: true},
	})
	if err != nil {
		t.Fatalf("create product: %v", err)
	}

	return int(userID64), int(productID64)
}

func TestCheckout_ValidationErrors(t *testing.T) {
	svc, store, cleanup := newTestService(t)
	defer cleanup()

	userID, _ := createUserAndProduct(t, store, 3)

	if _, err := svc.Checkout(userID, " "); err != ErrDeliveryAddress {
		t.Fatalf("delivery error = %v, want %v", err, ErrDeliveryAddress)
	}

	if _, err := svc.Checkout(userID, "Nairobi"); err != ErrCartEmpty {
		t.Fatalf("cart error = %v, want %v", err, ErrCartEmpty)
	}
}

func TestCalculateSummary(t *testing.T) {
	summary := CalculateSummary(2000)
	if summary.Subtotal != 2000 || summary.ShippingFee != 650 || summary.TaxAmount != 160 || summary.Total != 2810 {
		t.Fatalf("summary = %+v, want 20.00 + 6.50 + 1.60 = 28.10", summary)
	}

	free := CalculateSummary(8000)
	if free.ShippingFee != 0 || free.Total != 8640 {
		t.Fatalf("free shipping summary = %+v", free)
	}
}

func TestCheckoutWithPayment_InvalidMethod(t *testing.T) {
	svc, store, cleanup := newTestService(t)
	defer cleanup()

	userID, productID := createUserAndProduct(t, store, 3)
	if err := store.AddToCart(userID, productID, 1); err != nil {
		t.Fatalf("add to cart error = %v", err)
	}

	if _, _, err := svc.CheckoutWithPayment(userID, "Nairobi", "wire_transfer", ""); err != ErrInvalidPaymentMethod {
		t.Fatalf("error = %v, want %v", err, ErrInvalidPaymentMethod)
	}
}

func TestPrepare_CreatesCheckoutReservations(t *testing.T) {
	svc, store, cleanup := newTestService(t)
	defer cleanup()

	userID, productID := createUserAndProduct(t, store, 3)
	if err := store.AddToCart(userID, productID, 2); err != nil {
		t.Fatalf("add to cart error = %v", err)
	}

	key := "reservation-key"
	if err := svc.Prepare(userID, key); err != nil {
		t.Fatalf("prepare error = %v", err)
	}

	reservations, err := store.ListActiveStockReservationsByKey(userID, key)
	if err != nil {
		t.Fatalf("list reservations error = %v", err)
	}
	if len(reservations) != 1 {
		t.Fatalf("reservations length = %d, want 1", len(reservations))
	}
	if reservations[0].Quantity != 2 {
		t.Fatalf("reservation quantity = %d, want 2", reservations[0].Quantity)
	}

	product, err := store.GetProductByID(productID)
	if err != nil {
		t.Fatalf("get product error = %v", err)
	}
	if product.StockQuantity != 1 {
		t.Fatalf("projected stock = %d, want 1", product.StockQuantity)
	}

	checkout, err := store.GetCheckoutByToken(userID, key)
	if err != nil {
		t.Fatalf("get checkout error = %v", err)
	}
	if checkout == nil {
		t.Fatal("expected persisted checkout")
	}
	if checkout.Status != db.CheckoutStatusOpen {
		t.Fatalf("checkout status = %q, want %q", checkout.Status, db.CheckoutStatusOpen)
	}
	if len(checkout.Lines) != 1 {
		t.Fatalf("checkout lines length = %d, want 1", len(checkout.Lines))
	}
	if checkout.TotalAmount != 2810 {
		t.Fatalf("checkout total = %s, want 28.10", checkout.TotalAmount)
	}
}

func TestPreparedCheckout_ReturnsPersistedSnapshotState(t *testing.T) {
	svc, store, cleanup := newTestService(t)
	defer cleanup()

	userID, productID := createUserAndProduct(t, store, 4)
	if err := store.AddToCart(userID, productID, 2); err != nil {
		t.Fatalf("add to cart error = %v", err)
	}

	state, err := svc.PreparedCheckout(userID, "")
	if err != nil {
		t.Fatalf("prepared checkout error = %v", err)
	}
	if strings.TrimSpace(state.Token) == "" {
		t.Fatal("expected checkout token")
	}
	if !state.CanCheckout {
		t.Fatal("expected checkout to be allowed")
	}
	if len(state.Items) != 1 {
		t.Fatalf("items length = %d, want 1", len(state.Items))
	}
	if state.Items[0].Quantity != 2 {
		t.Fatalf("item quantity = %d, want 2", state.Items[0].Quantity)
	}
	if state.Summary.Total != 2810 {
		t.Fatalf("summary total = %s, want 28.10", state.Summary.Total)
	}

	checkout, err := store.GetCheckoutByToken(userID, state.Token)
	if err != nil {
		t.Fatalf("get checkout error = %v", err)
	}
	if checkout == nil {
		t.Fatal("expected persisted checkout")
	}
}

func TestSaveDraft_PersistsDeliveryAddressAndReusesOpenCheckout(t *testing.T) {
	svc, store, cleanup := newTestService(t)
	defer cleanup()

	userID, productID := createUserAndProduct(t, store, 4)
	if err := store.AddToCart(userID, productID, 2); err != nil {
		t.Fatalf("add to cart error = %v", err)
	}

	initialState, err := svc.PreparedCheckout(userID, "")
	if err != nil {
		t.Fatalf("prepared checkout error = %v", err)
	}

	draftState, err := svc.SaveDraft(userID, initialState.Token, "Saved Nairobi Lane", paymentsvc.MethodCardPlaceholder)
	if err != nil {
		t.Fatalf("save draft error = %v", err)
	}
	if draftState.Token != initialState.Token {
		t.Fatalf("draft token = %q, want %q", draftState.Token, initialState.Token)
	}
	if draftState.DeliveryAddress != "Saved Nairobi Lane" {
		t.Fatalf("draft address = %q, want %q", draftState.DeliveryAddress, "Saved Nairobi Lane")
	}
	if draftState.PaymentMethod != paymentsvc.MethodCardPlaceholder {
		t.Fatalf("draft payment method = %q, want %q", draftState.PaymentMethod, paymentsvc.MethodCardPlaceholder)
	}

	refreshedState, err := svc.PreparedCheckout(userID, "")
	if err != nil {
		t.Fatalf("refreshed checkout error = %v", err)
	}
	if refreshedState.Token != initialState.Token {
		t.Fatalf("refreshed token = %q, want %q", refreshedState.Token, initialState.Token)
	}
	if refreshedState.DeliveryAddress != "Saved Nairobi Lane" {
		t.Fatalf("refreshed address = %q, want %q", refreshedState.DeliveryAddress, "Saved Nairobi Lane")
	}
	if refreshedState.PaymentMethod != paymentsvc.MethodCardPlaceholder {
		t.Fatalf("refreshed payment method = %q, want %q", refreshedState.PaymentMethod, paymentsvc.MethodCardPlaceholder)
	}
}

func TestCheckoutWithPayment_PersistsComputedTotal(t *testing.T) {
	svc, store, cleanup := newTestService(t)
	defer cleanup()

	userID, productID := createUserAndProduct(t, store, 5)
	if err := store.AddToCart(userID, productID, 2); err != nil {
		t.Fatalf("add to cart error = %v", err)
	}

	orderID, summary, err := svc.CheckoutWithPayment(userID, "Nairobi", paymentsvc.MethodCardPlaceholder, "")
	if err != nil {
		t.Fatalf("checkout error = %v", err)
	}
	if orderID == 0 {
		t.Fatal("expected non-zero order id")
	}

	orders, err := store.ListOrdersByUser(userID)
	if err != nil {
		t.Fatalf("list orders error = %v", err)
	}
	if len(orders) != 1 {
		t.Fatalf("orders length = %d, want 1", len(orders))
	}
	if orders[0].TotalAmount != summary.Total {
		t.Fatalf("order total = %s, want %s", orders[0].TotalAmount, summary.Total)
	}
	if orders[0].DeliveryNotice == "" {
		t.Fatal("expected delivery notice to include payment/price context")
	}

	payments, err := store.ListPaymentsByOrderID(int(orderID))
	if err != nil {
		t.Fatalf("list payments error = %v", err)
	}
	if len(payments) != 1 {
		t.Fatalf("payments length = %d, want 1", len(payments))
	}
	if payments[0].Status != db.PaymentStatusCaptured {
		t.Fatalf("payment status = %q, want %q", payments[0].Status, db.PaymentStatusCaptured)
	}

	stocks, err := store.ListInventoryStocksByProductID(productID)
	if err != nil {
		t.Fatalf("list stocks error = %v", err)
	}
	if len(stocks) != 1 {
		t.Fatalf("stocks length = %d, want 1", len(stocks))
	}
	if stocks[0].ReservedQuantity != 0 {
		t.Fatalf("reserved quantity = %d, want 0", stocks[0].ReservedQuantity)
	}
	if stocks[0].AllocatedQuantity != 2 {
		t.Fatalf("allocated quantity = %d, want 2", stocks[0].AllocatedQuantity)
	}
}

func TestCheckoutWithPayment_ConvertsActiveReservations(t *testing.T) {
	svc, store, cleanup := newTestService(t)
	defer cleanup()

	userID, productID := createUserAndProduct(t, store, 5)
	if err := store.AddToCart(userID, productID, 2); err != nil {
		t.Fatalf("add to cart error = %v", err)
	}

	key := "convert-key"
	if err := svc.Prepare(userID, key); err != nil {
		t.Fatalf("prepare error = %v", err)
	}

	if _, _, err := svc.CheckoutWithPayment(userID, "Nairobi", paymentsvc.MethodCardPlaceholder, key); err != nil {
		t.Fatalf("checkout error = %v", err)
	}

	reservations, err := store.ListActiveStockReservationsByKey(userID, key)
	if err != nil {
		t.Fatalf("list reservations error = %v", err)
	}
	if len(reservations) != 0 {
		t.Fatalf("active reservations length = %d, want 0", len(reservations))
	}

	stocks, err := store.ListInventoryStocksByProductID(productID)
	if err != nil {
		t.Fatalf("list stocks error = %v", err)
	}
	if len(stocks) != 1 {
		t.Fatalf("stocks length = %d, want 1", len(stocks))
	}
	if stocks[0].ReservedQuantity != 0 {
		t.Fatalf("reserved quantity = %d, want 0", stocks[0].ReservedQuantity)
	}
	if stocks[0].AllocatedQuantity != 2 {
		t.Fatalf("allocated quantity = %d, want 2", stocks[0].AllocatedQuantity)
	}
}

func TestCheckoutWithPayment_IdempotentReplayReturnsExistingOrder(t *testing.T) {
	svc, store, cleanup := newTestService(t)
	defer cleanup()

	userID, productID := createUserAndProduct(t, store, 5)
	if err := store.AddToCart(userID, productID, 2); err != nil {
		t.Fatalf("add to cart error = %v", err)
	}

	key, err := paymentsvc.New().GenerateIdempotencyKey()
	if err != nil {
		t.Fatalf("generate key error = %v", err)
	}

	orderID, _, err := svc.CheckoutWithPayment(userID, "Nairobi", paymentsvc.MethodCardPlaceholder, key)
	if err != nil {
		t.Fatalf("first checkout error = %v", err)
	}
	replayedOrderID, replaySummary, err := svc.CheckoutWithPayment(userID, "Nairobi", paymentsvc.MethodCardPlaceholder, key)
	if err != nil {
		t.Fatalf("replay checkout error = %v", err)
	}
	if replayedOrderID != orderID {
		t.Fatalf("replayed order id = %d, want %d", replayedOrderID, orderID)
	}
	if replaySummary.Total != 2810 {
		t.Fatalf("replayed total = %s, want 28.10", replaySummary.Total)
	}

	orders, err := store.ListOrdersByUser(userID)
	if err != nil {
		t.Fatalf("list orders error = %v", err)
	}
	if len(orders) != 1 {
		t.Fatalf("orders length = %d, want 1", len(orders))
	}

	payments, err := store.ListPaymentsByOrderID(int(orderID))
	if err != nil {
		t.Fatalf("list payments error = %v", err)
	}
	if len(payments) != 1 {
		t.Fatalf("payments length = %d, want 1", len(payments))
	}
}

// Changing the cart after a checkout was prepared must not place an order for
// the stale snapshot; the checkout is re-prepared from the current cart.
func TestCheckoutWithPayment_RepricesAfterCartChange(t *testing.T) {
	svc, store, cleanup := newTestService(t)
	defer cleanup()

	userID, productID := createUserAndProduct(t, store, 5)
	if err := store.AddToCart(userID, productID, 2); err != nil {
		t.Fatalf("add to cart error = %v", err)
	}

	key := "snapshot-key"
	if err := svc.Prepare(userID, key); err != nil {
		t.Fatalf("prepare error = %v", err)
	}

	if err := store.UpdateCartQuantity(userID, productID, 1); err != nil {
		t.Fatalf("update cart quantity error = %v", err)
	}

	orderID, summary, err := svc.CheckoutWithPayment(userID, "Nairobi", paymentsvc.MethodCardPlaceholder, key)
	if err != nil {
		t.Fatalf("checkout error = %v", err)
	}
	if orderID == 0 {
		t.Fatal("expected non-zero order id")
	}
	if summary.Total != 1730 {
		t.Fatalf("summary total = %s, want 17.30", summary.Total)
	}

	orders, err := store.ListOrdersByUser(userID)
	if err != nil {
		t.Fatalf("list orders error = %v", err)
	}
	if len(orders) != 1 {
		t.Fatalf("orders length = %d, want 1", len(orders))
	}
	if len(orders[0].Items) != 1 {
		t.Fatalf("order items length = %d, want 1", len(orders[0].Items))
	}
	if orders[0].Items[0].Quantity != 1 {
		t.Fatalf("order quantity = %d, want 1", orders[0].Items[0].Quantity)
	}

	checkout, err := store.GetCheckoutByToken(userID, key)
	if err != nil {
		t.Fatalf("get checkout error = %v", err)
	}
	if checkout == nil {
		t.Fatal("expected checkout after completion")
	}
	if checkout.Status != db.CheckoutStatusCompleted {
		t.Fatalf("checkout status = %q, want %q", checkout.Status, db.CheckoutStatusCompleted)
	}
	if !checkout.OrderID.Valid || int(checkout.OrderID.Int64) != int(orderID) {
		t.Fatalf("checkout order id = %v, want %d", checkout.OrderID, orderID)
	}
}

func TestCheckoutWithPayment_CompletedTokenCannotBeReused(t *testing.T) {
	svc, store, cleanup := newTestService(t)
	defer cleanup()

	userID, productID := createUserAndProduct(t, store, 5)
	_ = store.AddToCart(userID, productID, 1)
	key := "single-use"
	if _, _, err := svc.CheckoutWithPayment(userID, "Nairobi", "", key); err != nil {
		t.Fatalf("first checkout: %v", err)
	}
	state, err := svc.PreparedCheckout(userID, key)
	if err != ErrCartEmpty {
		t.Fatalf("prepared completed checkout = %+v, %v; want ErrCartEmpty", state, err)
	}
}

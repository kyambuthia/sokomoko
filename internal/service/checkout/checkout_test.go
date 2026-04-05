package checkout

import (
	"database/sql"
	"math"
	"os"
	"testing"

	"github.com/kyambuthia/sokomoko/internal/db"
	paymentsvc "github.com/kyambuthia/sokomoko/internal/service/payment"
)

func newTestService(t *testing.T) (*Service, *db.Store, func()) {
	t.Helper()

	path := "./test_checkout_service.db"
	_ = os.Remove(path)

	store, err := db.OpenStore(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := store.ApplySchema(); err != nil {
		t.Fatalf("apply schema: %v", err)
	}

	cleanup := func() {
		_ = store.Close()
		_ = os.Remove(path)
	}

	return New(store, paymentsvc.New()), store, cleanup
}

func createUserAndProduct(t *testing.T, store *db.Store, stock int) (int, int) {
	t.Helper()

	userID64, err := store.CreateUser(db.User{
		Username:     "u_test",
		Email:        "u_test@example.com",
		PasswordHash: "hash",
		Salt:         "salt",
		Role:         "user",
		Slug:         "u-test",
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
		Price:         10.0,
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
	summary := CalculateSummary(20)
	if summary.Subtotal != 20 {
		t.Fatalf("subtotal = %.2f, want 20.00", summary.Subtotal)
	}
	if summary.ShippingFee != 6.50 {
		t.Fatalf("shipping = %.2f, want 6.50", summary.ShippingFee)
	}
	if summary.TaxAmount != 1.60 {
		t.Fatalf("tax = %.2f, want 1.60", summary.TaxAmount)
	}
	if summary.Total != 28.10 {
		t.Fatalf("total = %.2f, want 28.10", summary.Total)
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
	if math.Abs(orders[0].TotalAmount-summary.Total) > 0.001 {
		t.Fatalf("order total = %.2f, want %.2f", orders[0].TotalAmount, summary.Total)
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
	if math.Abs(replaySummary.Total-28.10) > 0.001 {
		t.Fatalf("replayed total = %.2f, want 28.10", replaySummary.Total)
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

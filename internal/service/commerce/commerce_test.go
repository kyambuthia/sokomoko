package commerce

import (
	"database/sql"
	"math"
	"os"
	"testing"

	"github.com/kyambuthia/sokomoko/internal/db"
)

func newTestService(t *testing.T) (*Service, *db.Store, func()) {
	t.Helper()

	path := "./test_commerce_service.db"
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

	return New(store), store, cleanup
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

func TestAddToCart_InvalidProduct(t *testing.T) {
	svc, store, cleanup := newTestService(t)
	defer cleanup()

	userID, _ := createUserAndProduct(t, store, 3)
	err := svc.AddToCart(userID, 0, 1)
	if err != ErrInvalidProduct {
		t.Fatalf("error = %v, want %v", err, ErrInvalidProduct)
	}
}

func TestAddToCart_OutOfStock(t *testing.T) {
	svc, store, cleanup := newTestService(t)
	defer cleanup()

	userID, productID := createUserAndProduct(t, store, 0)
	err := svc.AddToCart(userID, productID, 1)
	if err != ErrOutOfStock {
		t.Fatalf("error = %v, want %v", err, ErrOutOfStock)
	}
}

func TestAddToCart_InsufficientStock(t *testing.T) {
	svc, store, cleanup := newTestService(t)
	defer cleanup()

	userID, productID := createUserAndProduct(t, store, 2)
	if err := svc.AddToCart(userID, productID, 2); err != nil {
		t.Fatalf("initial add error = %v", err)
	}

	err := svc.AddToCart(userID, productID, 1)
	if err != ErrInsufficientStock {
		t.Fatalf("error = %v, want %v", err, ErrInsufficientStock)
	}
}

func TestUpdateCartItem_InsufficientStock(t *testing.T) {
	svc, store, cleanup := newTestService(t)
	defer cleanup()

	userID, productID := createUserAndProduct(t, store, 3)
	if err := svc.AddToCart(userID, productID, 1); err != nil {
		t.Fatalf("initial add error = %v", err)
	}

	err := svc.UpdateCartItem(userID, productID, 5)
	if err != ErrInsufficientStock {
		t.Fatalf("error = %v, want %v", err, ErrInsufficientStock)
	}
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

func TestCalculateCheckoutSummary(t *testing.T) {
	summary := CalculateCheckoutSummary(20)
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
	if err := svc.AddToCart(userID, productID, 1); err != nil {
		t.Fatalf("add to cart error = %v", err)
	}

	if _, _, err := svc.CheckoutWithPayment(userID, "Nairobi", "wire_transfer"); err != ErrInvalidPaymentMethod {
		t.Fatalf("error = %v, want %v", err, ErrInvalidPaymentMethod)
	}
}

func TestCheckoutWithPayment_PersistsComputedTotal(t *testing.T) {
	svc, store, cleanup := newTestService(t)
	defer cleanup()

	userID, productID := createUserAndProduct(t, store, 5)
	if err := svc.AddToCart(userID, productID, 2); err != nil {
		t.Fatalf("add to cart error = %v", err)
	}

	orderID, summary, err := svc.CheckoutWithPayment(userID, "Nairobi", PaymentMethodCardPlaceholder)
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
}

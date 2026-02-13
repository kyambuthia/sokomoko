package commerce

import (
	"database/sql"
	"os"
	"testing"

	"github.com/kyambuthia/sokomoko/internal/db"
)

func newTestService(t *testing.T) (*Service, func()) {
	t.Helper()

	path := "./test_commerce_service.db"
	_ = os.Remove(path)

	store, err := db.OpenStoreNoSeed(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}

	cleanup := func() {
		_ = store.Close()
		_ = os.Remove(path)
	}

	return New(store), cleanup
}

func createUserAndProduct(t *testing.T, svc *Service, stock int) (int, int) {
	t.Helper()

	userID64, err := svc.store.CreateUser(db.User{
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

	catID, err := svc.store.CreateCategory(db.Category{
		Name: "Cat A",
		Slug: "cat-a",
	})
	if err != nil {
		t.Fatalf("create category: %v", err)
	}

	productID64, err := svc.store.CreateProduct(db.Product{
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
	svc, cleanup := newTestService(t)
	defer cleanup()

	userID, _ := createUserAndProduct(t, svc, 3)
	err := svc.AddToCart(userID, 0, 1)
	if err != ErrInvalidProduct {
		t.Fatalf("error = %v, want %v", err, ErrInvalidProduct)
	}
}

func TestAddToCart_OutOfStock(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()

	userID, productID := createUserAndProduct(t, svc, 0)
	err := svc.AddToCart(userID, productID, 1)
	if err != ErrOutOfStock {
		t.Fatalf("error = %v, want %v", err, ErrOutOfStock)
	}
}

func TestCheckout_ValidationErrors(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()

	userID, _ := createUserAndProduct(t, svc, 3)

	if _, err := svc.Checkout(userID, " "); err != ErrDeliveryAddress {
		t.Fatalf("delivery error = %v, want %v", err, ErrDeliveryAddress)
	}

	if _, err := svc.Checkout(userID, "Nairobi"); err != ErrCartEmpty {
		t.Fatalf("cart error = %v, want %v", err, ErrCartEmpty)
	}
}

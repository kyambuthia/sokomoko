package commerce

import (
	"database/sql"
	"testing"
	"time"

	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/db/dbtest"
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

	return New(store), store, cleanup
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

func TestAddToCart_OwnCheckoutHoldCountsAsAvailable(t *testing.T) {
	svc, store, cleanup := newTestService(t)
	defer cleanup()

	userID, productID := createUserAndProduct(t, store, 3)
	if err := svc.AddToCart(userID, productID, 2); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := store.ReserveCartForCheckout(userID, "hold", time.Now().Add(time.Minute)); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if err := svc.AddToCart(userID, productID, 1); err != nil {
		t.Fatalf("adding the last unit while holding the rest: %v", err)
	}
	cart, err := svc.Cart(userID)
	if err != nil || cart.ItemCount != 3 || cart.Subtotal != 3000 {
		t.Fatalf("cart = %+v, %v", cart, err)
	}
}

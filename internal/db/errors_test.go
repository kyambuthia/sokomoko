package db

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestCreateProduct_DuplicateSlug_ReturnsTypedConflict(t *testing.T) {
	store, cleanup := openIsolatedStore(t)
	defer cleanup()

	suffix := "typed-slug"
	categoryID, err := store.CreateCategory(Category{
		Name: "Typed Error Category",
		Slug: "typed-error-category-" + suffix,
	})
	if err != nil {
		t.Fatalf("create category failed: %v", err)
	}

	product := Product{
		Name:          "Typed Product",
		Slug:          "typed-product-" + suffix,
		Description:   "typed product",
		Price:         10,
		StockQuantity: 2,
		CategoryID:    sql.NullInt64{Int64: categoryID, Valid: true},
	}
	if _, err := store.CreateProduct(product); err != nil {
		t.Fatalf("initial create product failed: %v", err)
	}

	_, err = store.CreateProduct(product)
	if !errors.Is(err, ErrProductSlugConflict) {
		t.Fatalf("expected ErrProductSlugConflict, got %v", err)
	}
}

func TestPlaceOrderFromCartWithPricing_TypedErrors(t *testing.T) {
	store, cleanup := openIsolatedStore(t)
	defer cleanup()

	suffix := time.Now().UnixNano()
	userID, err := store.CreateUser(User{
		Username:     fmt.Sprintf("typed_user_%d", suffix),
		Email:        fmt.Sprintf("typed_user_%d@example.com", suffix),
		PasswordHash: "hash",
		Salt:         "salt",
		Role:         "user",
		Slug:         fmt.Sprintf("typed-user-%d", suffix),
	})
	if err != nil {
		t.Fatalf("create user failed: %v", err)
	}

	if _, err := store.PlaceOrderFromCartWithPricing(int(userID), " ", 10, "notice"); !errors.Is(err, ErrDeliveryAddressRequired) {
		t.Fatalf("expected ErrDeliveryAddressRequired, got %v", err)
	}
	if _, err := store.PlaceOrderFromCartWithPricing(int(userID), "Nairobi", 10, "notice"); !errors.Is(err, ErrCartEmpty) {
		t.Fatalf("expected ErrCartEmpty, got %v", err)
	}
}

func openIsolatedStore(t *testing.T) (*Store, func()) {
	t.Helper()

	path := fmt.Sprintf("./test_typed_errors_%d.db", time.Now().UnixNano())
	store, err := OpenStore(path)
	if err != nil {
		t.Fatalf("open isolated store failed: %v", err)
	}
	if err := store.ApplySchema(); err != nil {
		t.Fatalf("apply schema failed: %v", err)
	}

	cleanup := func() {
		_ = store.Close()
		_ = os.Remove(path)
	}

	return store, cleanup
}

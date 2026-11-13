package db

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestDeleteProductForActor_OwnershipCartCheckoutCleanupAndConcurrency(t *testing.T) {
	s := newTestStore(t)
	_, owner1, product1 := chatShop(t, s)
	_, outsider, _ := chatShop(t, s)
	buyer := mustCreateUser(t, s, RoleUser)

	// Outsider partner cannot delete owner1's product.
	if err := s.DeleteProductForActor(t.Context(), product1, outsider, RolePartner); !errors.Is(err, ErrProductNotFound) {
		t.Fatalf("outsider delete err = %v; want ErrProductNotFound", err)
	}

	// Add product1 to buyer's cart and reserve for checkout.
	chatMust(t, s.AddToCart(t.Context(), buyer, product1, 1))
	chatMust(t, s.ReserveCartForCheckout(t.Context(), buyer, "del-res-key", time.Now().UTC().Add(5*time.Minute)))

	// Owner deletes product1: cart items and checkout reservations are cleaned up, product is soft-deleted.
	chatMust(t, s.DeleteProductForActor(t.Context(), product1, owner1, RolePartner))

	items, _, err := s.GetCartItems(t.Context(), buyer)
	chatMust(t, err)
	if len(items) != 0 {
		t.Fatalf("cart items after product delete = %d; want 0", len(items))
	}

	if err := s.AddToCart(t.Context(), buyer, product1, 1); !errors.Is(err, ErrProductNotFound) {
		t.Fatalf("AddToCart after delete err = %v; want ErrProductNotFound", err)
	}

	// Concurrent AddToCart and DeleteProductForActor on a second product complete cleanly without races or deadlocks.
	product2ID, err := s.CreateProductForActor(t.Context(), Product{
		Name:          "Concurrent Delete Item",
		Slug:          "concurrent-delete-item",
		Description:   "Test item",
		Price:         500,
		Currency:      "USD",
		StockQuantity: 10,
	}, owner1, RolePartner)
	chatMust(t, err)
	product2 := int(product2ID)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = s.AddToCart(t.Context(), buyer, product2, 1)
	}()
	go func() {
		defer wg.Done()
		_ = s.DeleteProductForActor(t.Context(), product2, owner1, RolePartner)
	}()
	wg.Wait()

	remaining, _, err := s.GetCartItems(t.Context(), buyer)
	chatMust(t, err)
	if len(remaining) != 0 {
		t.Fatalf("cart items after concurrent delete = %d; want 0", len(remaining))
	}
}

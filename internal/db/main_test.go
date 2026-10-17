package db

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kyambuthia/sokomoko/internal/db/dbtest"
	"github.com/kyambuthia/sokomoko/internal/money"
)

// newTestStore returns a migrated store in a private schema.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := OpenStore(dbtest.DSN(t))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.ApplySchema(); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	return store
}

var fixtureSeq atomic.Int64

func nextSuffix() string {
	return fmt.Sprintf("%d%d", time.Now().UnixNano()%1e6, fixtureSeq.Add(1))
}

func mustCreateUser(t *testing.T, s *Store, role string) int {
	t.Helper()
	suffix := nextSuffix()
	id, err := s.CreateUser(User{
		Username:     "user" + suffix,
		Email:        "user" + suffix + "@example.com",
		PasswordHash: "hash",
		Role:         role,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	return int(id)
}

func mustCreateProduct(t *testing.T, s *Store, price money.Cents, stock int) int {
	t.Helper()
	suffix := nextSuffix()
	id, err := s.CreateProduct(Product{
		Name:          "Product " + suffix,
		Slug:          "product-" + suffix,
		Description:   "test product",
		Price:         price,
		StockQuantity: stock,
	})
	if err != nil {
		t.Fatalf("create product: %v", err)
	}
	return int(id)
}

func mustStock(t *testing.T, s *Store, productID int) InventoryStock {
	t.Helper()
	stocks, err := s.ListInventoryStocksByProductID(productID)
	if err != nil {
		t.Fatalf("list stocks: %v", err)
	}
	if len(stocks) != 1 {
		t.Fatalf("stocks = %d rows, want 1", len(stocks))
	}
	return stocks[0]
}

func mustAvailable(t *testing.T, s *Store, productID int) int {
	t.Helper()
	product, err := s.GetProductByID(productID)
	if err != nil || product == nil {
		t.Fatalf("get product %d: %v", productID, err)
	}
	return product.StockQuantity
}

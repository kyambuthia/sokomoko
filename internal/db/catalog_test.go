package db

import (
	"database/sql"
	"errors"
	"testing"
	"time"
)

func TestCreateProduct_InitializesInventory(t *testing.T) {
	s := newTestStore(t)
	partnerID, err := s.UpsertPartner(Partner{Name: "Acme", Slug: "acme"})
	if err != nil {
		t.Fatalf("partner: %v", err)
	}
	catID, err := s.CreateCategory(Category{Name: "Audio", Slug: "audio"})
	if err != nil {
		t.Fatalf("category: %v", err)
	}

	id, err := s.CreateProduct(Product{
		Name:          "Speaker",
		Slug:          "speaker",
		Price:         4999,
		StockQuantity: 7,
		CategoryID:    sql.NullInt64{Int64: catID, Valid: true},
		PartnerID:     sql.NullInt64{Int64: partnerID, Valid: true},
		Images:        []ProductImage{{URL: "/static/a.png"}, {URL: "/static/b.png"}},
	})
	if err != nil {
		t.Fatalf("create product: %v", err)
	}

	product, err := s.GetProductBySlug("speaker")
	if err != nil || product == nil {
		t.Fatalf("get by slug: %v", err)
	}
	if product.ID != int(id) || product.Price != 4999 || product.StockQuantity != 7 {
		t.Fatalf("product = %+v", product)
	}
	if product.Category != "Audio" || product.PartnerName != "Acme" {
		t.Fatalf("joins: category=%q partner=%q", product.Category, product.PartnerName)
	}
	if len(product.Images) != 2 || product.PrimaryImage() != "/static/a.png" {
		t.Fatalf("images = %+v", product.Images)
	}

	stock := mustStock(t, s, int(id))
	if stock.OnHandQuantity != 7 || stock.ReservedQuantity != 0 || stock.AllocatedQuantity != 0 {
		t.Fatalf("stock = %+v", stock)
	}
	movements, _ := s.ListStockMovementsByProductID(int(id))
	if len(movements) != 1 || movements[0].MovementType != stockMovementInitial || movements[0].QuantityDelta != 7 {
		t.Fatalf("movements = %+v", movements)
	}
}

func TestCreateProduct_DuplicateSlugIsTypedConflict(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateProduct(Product{Name: "A", Slug: "dup", Price: 100}); err != nil {
		t.Fatalf("first: %v", err)
	}
	_, err := s.CreateProduct(Product{Name: "B", Slug: "dup", Price: 100})
	if !errors.Is(err, ErrProductSlugConflict) {
		t.Fatalf("err = %v, want ErrProductSlugConflict", err)
	}

	// A soft-deleted product frees its slug.
	first, _ := s.GetProductBySlug("dup")
	if err := s.DeleteProduct(first.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.CreateProduct(Product{Name: "C", Slug: "dup", Price: 100}); err != nil {
		t.Fatalf("recreate after delete: %v", err)
	}
}

func TestUpdateProduct_SetsAvailableAroundHolds(t *testing.T) {
	s := newTestStore(t)
	userID := mustCreateUser(t, s, RoleUser)
	productID := mustCreateProduct(t, s, 1000, 10)

	if err := s.AddToCart(userID, productID, 3); err != nil {
		t.Fatalf("add to cart: %v", err)
	}
	if err := s.ReserveCartForCheckout(userID, "hold", timeNow().Add(time.Minute)); err != nil {
		t.Fatalf("reserve: %v", err)
	}

	product, _ := s.GetProductByID(productID)
	product.Name = "Renamed"
	product.Price = 1250
	product.StockQuantity = 4
	if err := s.UpdateProduct(*product); err != nil {
		t.Fatalf("update: %v", err)
	}

	stock := mustStock(t, s, productID)
	if stock.AvailableQuantity != 4 || stock.ReservedQuantity != 3 || stock.OnHandQuantity != 7 {
		t.Fatalf("stock = %+v", stock)
	}
	updated, _ := s.GetProductByID(productID)
	if updated.Name != "Renamed" || updated.Price != 1250 {
		t.Fatalf("product = %+v", updated)
	}

	if err := s.UpdateProduct(Product{ID: 999999, Name: "x", Slug: "x"}); !errors.Is(err, ErrProductNotFound) {
		t.Fatalf("missing product err = %v", err)
	}
}

func TestListProducts_SearchAndFilters(t *testing.T) {
	s := newTestStore(t)
	audio, _ := s.CreateCategory(Category{Name: "Audio", Slug: "audio"})
	home, _ := s.CreateCategory(Category{Name: "Home & Kitchen", Slug: "home-kitchen"})
	mk := func(name, slug, desc string, cat int64, stock int) {
		t.Helper()
		if _, err := s.CreateProduct(Product{Name: name, Slug: slug, Description: desc, Price: 100, StockQuantity: stock,
			CategoryID: sql.NullInt64{Int64: cat, Valid: true}}); err != nil {
			t.Fatalf("create %s: %v", slug, err)
		}
	}
	mk("Wireless Headphones", "wireless-headphones", "Over-ear audio", audio, 3)
	mk("Bluetooth Speaker", "bluetooth-speaker", "Portable wireless sound", audio, 0)
	mk("Electric Kettle", "electric-kettle", "Stainless 100% steel", home, 5)

	cases := []struct {
		name   string
		filter ProductFilter
		want   int
	}{
		{"case insensitive substring", ProductFilter{Query: "HEADPH"}, 1},
		{"full text across description", ProductFilter{Query: "wireless"}, 2},
		{"category by slug", ProductFilter{Category: "audio"}, 2},
		{"category by name", ProductFilter{Category: "home & kitchen"}, 1},
		{"query within category", ProductFilter{Query: "wireless", Category: "Audio"}, 2},
		{"in stock only", ProductFilter{Category: "audio", InStockOnly: true}, 1},
		{"like wildcards are literal", ProductFilter{Query: "%"}, 1},
		{"limit", ProductFilter{Limit: 2}, 2},
	}
	for _, tc := range cases {
		got, err := s.ListProducts(tc.filter)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if len(got) != tc.want {
			t.Errorf("%s: got %d products, want %d", tc.name, len(got), tc.want)
		}
	}

	if results, _ := s.SearchProducts("   "); len(results) != 0 {
		t.Fatalf("blank search returned %d", len(results))
	}
}

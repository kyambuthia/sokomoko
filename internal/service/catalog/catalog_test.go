package catalog

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/money"
)

type mockCatalogStore struct {
	getAllCategoriesFn                 func(ctx context.Context) ([]db.Category, error)
	getPublicProductBySlugFn           func(ctx context.Context, slug string) (*db.Product, error)
	getPublicProductImageByIDFn        func(ctx context.Context, id int) (*db.ProductImage, error)
	getPublicProductVideoByProductIDFn func(ctx context.Context, productID int) (*db.ProductVideo, error)
	listProductsFn                     func(ctx context.Context, filter db.ProductFilter) ([]db.Product, error)
	listProductsPageFn                 func(ctx context.Context, filter db.ProductFilter) ([]db.Product, int, error)
	countFilteredProductsFn            func(ctx context.Context, filter db.ProductFilter) (int, error)
	getPartnerBySlugFn                 func(ctx context.Context, slug string) (*db.Partner, error)
}

func (m *mockCatalogStore) GetAllCategories(ctx context.Context) ([]db.Category, error) {
	if m.getAllCategoriesFn != nil {
		return m.getAllCategoriesFn(ctx)
	}
	return nil, nil
}

func (m *mockCatalogStore) GetPublicProductBySlug(ctx context.Context, slug string) (*db.Product, error) {
	if m.getPublicProductBySlugFn != nil {
		return m.getPublicProductBySlugFn(ctx, slug)
	}
	return nil, nil
}

func (m *mockCatalogStore) GetPublicProductImageByID(ctx context.Context, id int) (*db.ProductImage, error) {
	if m.getPublicProductImageByIDFn != nil {
		return m.getPublicProductImageByIDFn(ctx, id)
	}
	return nil, nil
}

func (m *mockCatalogStore) GetPublicProductVideoByProductID(ctx context.Context, productID int) (*db.ProductVideo, error) {
	if m.getPublicProductVideoByProductIDFn != nil {
		return m.getPublicProductVideoByProductIDFn(ctx, productID)
	}
	return nil, nil
}

func (m *mockCatalogStore) ListProducts(ctx context.Context, filter db.ProductFilter) ([]db.Product, error) {
	if m.listProductsFn != nil {
		return m.listProductsFn(ctx, filter)
	}
	return nil, nil
}

func (m *mockCatalogStore) ListProductsPage(ctx context.Context, filter db.ProductFilter) ([]db.Product, int, error) {
	if m.listProductsPageFn != nil {
		return m.listProductsPageFn(ctx, filter)
	}
	return nil, 0, nil
}

func (m *mockCatalogStore) CountFilteredProducts(ctx context.Context, filter db.ProductFilter) (int, error) {
	if m.countFilteredProductsFn != nil {
		return m.countFilteredProductsFn(ctx, filter)
	}
	return 0, nil
}

func (m *mockCatalogStore) GetPartnerBySlug(ctx context.Context, slug string) (*db.Partner, error) {
	if m.getPartnerBySlugFn != nil {
		return m.getPartnerBySlugFn(ctx, slug)
	}
	return nil, nil
}

func TestProductBySlug(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid slug formats rejected early", func(t *testing.T) {
		svc := New(&mockCatalogStore{
			getPublicProductBySlugFn: func(ctx context.Context, slug string) (*db.Product, error) {
				t.Fatal("store must not be called for invalid slug")
				return nil, nil
			},
		})

		invalidSlugs := []string{
			"",
			"   ",
			"has/slash",
			strings.Repeat("a", 201),
		}
		for _, s := range invalidSlugs {
			_, err := svc.ProductBySlug(ctx, s)
			if !errors.Is(err, ErrInvalidProductSlug) {
				t.Fatalf("ProductBySlug(%q) expected ErrInvalidProductSlug, got %v", s, err)
			}
		}
	})

	t.Run("valid slug mapped", func(t *testing.T) {
		svc := New(&mockCatalogStore{
			getPublicProductBySlugFn: func(ctx context.Context, slug string) (*db.Product, error) {
				if slug != "blue-shirt" {
					return nil, db.ErrProductNotFound
				}
				return &db.Product{
					ID:            42,
					Name:          "Blue Shirt",
					Slug:          "blue-shirt",
					Description:   "Cotton shirt",
					Price:         money.Cents(2500),
					Currency:      "KES",
					StockQuantity: 10,
					Category:      "Clothing",
					PartnerName:   "Acme Store",
					PartnerSlug:   "acme",
					Images: []db.ProductImage{
						{ID: 1, ProductID: 42, AltText: "Shirt front"},
					},
				}, nil
			},
		})

		p, err := svc.ProductBySlug(ctx, "blue-shirt")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p == nil || p.ID != 42 || p.Name != "Blue Shirt" || p.ImageAlt != "Shirt front" {
			t.Fatalf("product mapped incorrectly: %+v", p)
		}
		if !p.InStock() {
			t.Fatal("stock quantity 10 should be in stock")
		}
	})
}

func TestActiveStore(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid slugs return nil without touching store", func(t *testing.T) {
		svc := New(&mockCatalogStore{
			getPartnerBySlugFn: func(ctx context.Context, slug string) (*db.Partner, error) {
				t.Fatal("store must not be called")
				return nil, nil
			},
		})

		for _, s := range []string{"", "   ", "Store With Spaces", "has/slash", strings.Repeat("x", 85)} {
			partner, err := svc.ActiveStore(ctx, s)
			if err != nil || partner != nil {
				t.Fatalf("ActiveStore(%q) expected nil, nil; got %v, %v", s, partner, err)
			}
		}
	})

	t.Run("suspended partner returns nil", func(t *testing.T) {
		svc := New(&mockCatalogStore{
			getPartnerBySlugFn: func(ctx context.Context, slug string) (*db.Partner, error) {
				return &db.Partner{Slug: slug, Status: "suspended"}, nil
			},
		})
		partner, err := svc.ActiveStore(ctx, "my-shop")
		if err != nil || partner != nil {
			t.Fatalf("expected nil, nil for suspended partner; got %v, %v", partner, err)
		}
	})

	t.Run("active partner returned", func(t *testing.T) {
		svc := New(&mockCatalogStore{
			getPartnerBySlugFn: func(ctx context.Context, slug string) (*db.Partner, error) {
				return &db.Partner{Slug: slug, Status: "active"}, nil
			},
		})
		partner, err := svc.ActiveStore(ctx, "my-shop")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if partner == nil || partner.Slug != "my-shop" {
			t.Fatalf("expected active partner, got %+v", partner)
		}
	})
}

func TestPageAndBrowse(t *testing.T) {
	ctx := context.Background()

	t.Run("Page bounds pagination and offset", func(t *testing.T) {
		var capturedFilter db.ProductFilter
		svc := New(&mockCatalogStore{
			listProductsPageFn: func(ctx context.Context, filter db.ProductFilter) ([]db.Product, int, error) {
				capturedFilter = filter
				return []db.Product{{ID: 1, Name: "Item 1", Price: 100}}, 50, nil
			},
		})

		// Test page < 1 and limit > 50 clamping
		products, total, err := svc.Page(ctx, 0, 100)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if total != 50 || len(products) != 1 {
			t.Fatalf("Page result unexpected: total=%d, len=%d", total, len(products))
		}
		if capturedFilter.Limit != 50 || capturedFilter.Offset != 0 {
			t.Fatalf("expected Limit=50 Offset=0, got Limit=%d Offset=%d", capturedFilter.Limit, capturedFilter.Offset)
		}

		// Test page 3, limit 10 -> offset 20
		_, _, _ = svc.Page(ctx, 3, 10)
		if capturedFilter.Limit != 10 || capturedFilter.Offset != 20 {
			t.Fatalf("expected Limit=10 Offset=20, got Limit=%d Offset=%d", capturedFilter.Limit, capturedFilter.Offset)
		}
	})

	t.Run("Browse normalizes query and handles All Categories", func(t *testing.T) {
		var capturedFilter db.ProductFilter
		svc := New(&mockCatalogStore{
			listProductsFn: func(ctx context.Context, filter db.ProductFilter) ([]db.Product, error) {
				capturedFilter = filter
				return nil, nil
			},
		})

		longQuery := "  word1   word2   " + strings.Repeat("a", 150)
		_, err := svc.Browse(ctx, Filter{
			Query:    longQuery,
			Category: "All Departments",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if capturedFilter.Category != "" {
			t.Fatalf("expected empty category for All Departments, got %q", capturedFilter.Category)
		}
		if len([]rune(capturedFilter.Query)) > maxQueryLength {
			t.Fatalf("query length %d exceeds maxQueryLength %d", len([]rune(capturedFilter.Query)), maxQueryLength)
		}
	})

	t.Run("ProductCount skips negative partner IDs", func(t *testing.T) {
		svc := New(&mockCatalogStore{
			countFilteredProductsFn: func(ctx context.Context, filter db.ProductFilter) (int, error) {
				t.Fatal("store must not be called")
				return 0, nil
			},
		})
		count, err := svc.ProductCount(ctx, Filter{PartnerID: -1})
		if err != nil || count != 0 {
			t.Fatalf("expected 0, nil; got %d, %v", count, err)
		}
	})
}

func TestSearch(t *testing.T) {
	ctx := context.Background()

	t.Run("blank query returns empty without touching store", func(t *testing.T) {
		svc := New(&mockCatalogStore{
			listProductsFn: func(ctx context.Context, filter db.ProductFilter) ([]db.Product, error) {
				t.Fatal("store must not be called for blank search")
				return nil, nil
			},
		})

		results, err := svc.Search(ctx, "   \t\n  ")
		if err != nil || len(results) != 0 {
			t.Fatalf("expected empty results, got %v, err=%v", results, err)
		}
	})

	t.Run("SearchPage with blank query and category returns empty", func(t *testing.T) {
		svc := New(&mockCatalogStore{
			listProductsPageFn: func(ctx context.Context, filter db.ProductFilter) ([]db.Product, int, error) {
				t.Fatal("store must not be called")
				return nil, 0, nil
			},
		})

		results, total, err := svc.SearchPage(ctx, "   ", "all", 1, 20)
		if err != nil || len(results) != 0 || total != 0 {
			t.Fatalf("expected empty, got len=%d total=%d err=%v", len(results), total, err)
		}
	})
}

func TestProductImageAndVideo(t *testing.T) {
	ctx := context.Background()

	t.Run("non-positive IDs rejected before store", func(t *testing.T) {
		svc := New(&mockCatalogStore{
			getPublicProductImageByIDFn: func(ctx context.Context, id int) (*db.ProductImage, error) {
				t.Fatal("store should not be called")
				return nil, nil
			},
			getPublicProductVideoByProductIDFn: func(ctx context.Context, productID int) (*db.ProductVideo, error) {
				t.Fatal("store should not be called")
				return nil, nil
			},
		})

		if _, err := svc.ProductImage(ctx, 0); !errors.Is(err, db.ErrProductNotFound) {
			t.Fatalf("expected ErrProductNotFound, got %v", err)
		}
		if _, err := svc.ProductImage(ctx, -5); !errors.Is(err, db.ErrProductNotFound) {
			t.Fatalf("expected ErrProductNotFound, got %v", err)
		}
		if _, err := svc.ProductVideo(ctx, 0); !errors.Is(err, db.ErrProductNotFound) {
			t.Fatalf("expected ErrProductNotFound, got %v", err)
		}
		if _, err := svc.ProductVideo(ctx, -1); !errors.Is(err, db.ErrProductNotFound) {
			t.Fatalf("expected ErrProductNotFound, got %v", err)
		}
	})
}

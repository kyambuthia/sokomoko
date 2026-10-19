package catalog

import (
	"context"

	"github.com/kyambuthia/sokomoko/internal/db"
)

type store interface {
	CountPublicProducts(ctx context.Context) (int, error)
	GetAllCategories(ctx context.Context) ([]db.Category, error)
	GetPublicProductBySlug(ctx context.Context, slug string) (*db.Product, error)
	ListProducts(context.Context, db.ProductFilter) ([]db.Product, error)
	CountFilteredProducts(context.Context, db.ProductFilter) (int, error)
}

var _ store = (*db.Store)(nil)

package catalog

import (
	"context"

	"github.com/kyambuthia/sokomoko/internal/db"
)

type store interface {
	GetAllCategories(ctx context.Context) ([]db.Category, error)
	GetPublicProductBySlug(ctx context.Context, slug string) (*db.Product, error)
	GetPublicProductImageByID(ctx context.Context, id int) (*db.ProductImage, error)
	ListProducts(context.Context, db.ProductFilter) ([]db.Product, error)
	ListProductsPage(context.Context, db.ProductFilter) ([]db.Product, int, error)
}

var _ store = (*db.Store)(nil)

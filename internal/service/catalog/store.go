package catalog

import (
	"context"

	"github.com/kyambuthia/sokomoko/internal/db"
)

type store interface {
	CountPublicProducts() (int, error)
	GetAllCategories() ([]db.Category, error)
	GetPublicProductBySlug(slug string) (*db.Product, error)
	ListProducts(filter db.ProductFilter) ([]db.Product, error)
	ListProductsContext(context.Context, db.ProductFilter) ([]db.Product, error)
	CountFilteredProductsContext(context.Context, db.ProductFilter) (int, error)
}

var _ store = (*db.Store)(nil)

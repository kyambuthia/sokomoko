package catalog

import (
	"context"

	"github.com/kyambuthia/sokomoko/internal/db"
)

type store interface {
	GetAllCategories(ctx context.Context) ([]db.Category, error)
	GetPublicProductBySlug(ctx context.Context, slug string) (*db.Product, error)
	GetPublicProductImageByID(ctx context.Context, id int) (*db.ProductImage, error)
	GetPublicProductVideoByProductID(ctx context.Context, productID int) (*db.ProductVideo, error)
	ListProducts(context.Context, db.ProductFilter) ([]db.Product, error)
	ListProductsPage(context.Context, db.ProductFilter) ([]db.Product, int, error)
	CountFilteredProducts(context.Context, db.ProductFilter) (int, error)
	GetPartnerBySlug(context.Context, string) (*db.Partner, error)
}

var _ store = (*db.Store)(nil)

package catalog

import "github.com/kyambuthia/sokomoko/internal/db"

type store interface {
	CountProducts() (int, error)
	GetAllCategories() ([]db.Category, error)
	GetProductBySlug(slug string) (*db.Product, error)
	ListProducts(filter db.ProductFilter) ([]db.Product, error)
}

var _ store = (*db.Store)(nil)

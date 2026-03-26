package catalog

import "github.com/kyambuthia/sokomoko/internal/db"

type store interface {
	GetAllProducts() ([]Product, error)
	GetProductBySlug(slug string) (*Product, error)
	SearchProducts(query string) ([]Product, error)
}

type dbStore struct {
	store *db.Store
}

func newDBStore(store *db.Store) store {
	return &dbStore{store: store}
}

func (s *dbStore) GetAllProducts() ([]Product, error) {
	products, err := s.store.GetAllProducts()
	if err != nil {
		return nil, err
	}
	return mapProducts(products), nil
}

func (s *dbStore) GetProductBySlug(slug string) (*Product, error) {
	product, err := s.store.GetProductBySlug(slug)
	if err != nil || product == nil {
		return nil, err
	}
	mapped := mapProduct(*product)
	return &mapped, nil
}

func (s *dbStore) SearchProducts(query string) ([]Product, error) {
	products, err := s.store.SearchProducts(query)
	if err != nil {
		return nil, err
	}
	return mapProducts(products), nil
}

package catalog

import (
	"errors"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/db"
)

var ErrInvalidProductSlug = errors.New("invalid product slug")

type Service struct {
	store store
}

type store interface {
	GetAllProducts() ([]db.Product, error)
	GetProductBySlug(slug string) (*db.Product, error)
	SearchProducts(query string) ([]db.Product, error)
}

func New(store store) *Service {
	return &Service{store: store}
}

func (s *Service) AllProducts() ([]db.Product, error) {
	return s.store.GetAllProducts()
}

func (s *Service) ProductBySlug(slug string) (*db.Product, error) {
	cleanSlug := strings.TrimSpace(slug)
	if cleanSlug == "" || strings.Contains(cleanSlug, "/") {
		return nil, ErrInvalidProductSlug
	}
	return s.store.GetProductBySlug(cleanSlug)
}

func (s *Service) Search(query string) ([]db.Product, error) {
	cleanQuery := strings.TrimSpace(query)
	if cleanQuery == "" {
		return []db.Product{}, nil
	}
	return s.store.SearchProducts(cleanQuery)
}

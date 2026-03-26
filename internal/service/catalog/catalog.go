package catalog

import (
	"errors"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/db"
)

var ErrInvalidProductSlug = errors.New("invalid product slug")

type Product struct {
	ID            int
	Name          string
	Slug          string
	Description   string
	Price         float64
	StockQuantity int
	Category      string
	PrimaryImage  string
}

type Service struct {
	store store
}

func New(store *db.Store) *Service {
	return &Service{store: newDBStore(store)}
}

func newWithStore(store store) *Service {
	return &Service{store: store}
}

func (s *Service) AllProducts() ([]Product, error) {
	return s.store.GetAllProducts()
}

func (s *Service) ProductBySlug(slug string) (*Product, error) {
	cleanSlug := strings.TrimSpace(slug)
	if cleanSlug == "" || strings.Contains(cleanSlug, "/") {
		return nil, ErrInvalidProductSlug
	}
	product, err := s.store.GetProductBySlug(cleanSlug)
	return product, err
}

func (s *Service) Search(query string) ([]Product, error) {
	cleanQuery := strings.TrimSpace(query)
	if cleanQuery == "" {
		return []Product{}, nil
	}
	return s.store.SearchProducts(cleanQuery)
}

func (p Product) GetPrimaryImageURL() string {
	if p.PrimaryImage != "" {
		return p.PrimaryImage
	}
	return "/static/images/placeholder.png"
}

func mapProducts(products []db.Product) []Product {
	mapped := make([]Product, 0, len(products))
	for _, product := range products {
		mapped = append(mapped, mapProduct(product))
	}
	return mapped
}

func mapProduct(product db.Product) Product {
	return Product{
		ID:            product.ID,
		Name:          product.Name,
		Slug:          product.Slug,
		Description:   product.Description,
		Price:         product.Price,
		StockQuantity: product.StockQuantity,
		Category:      product.Category,
		PrimaryImage:  product.GetPrimaryImageURL(),
	}
}

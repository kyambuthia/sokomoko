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

type store interface {
	GetAllProducts() ([]db.Product, error)
	GetProductBySlug(slug string) (*db.Product, error)
	SearchProducts(query string) ([]db.Product, error)
}

func New(store store) *Service {
	return &Service{store: store}
}

func (s *Service) AllProducts() ([]Product, error) {
	products, err := s.store.GetAllProducts()
	if err != nil {
		return nil, err
	}
	return mapProducts(products), nil
}

func (s *Service) ProductBySlug(slug string) (*Product, error) {
	cleanSlug := strings.TrimSpace(slug)
	if cleanSlug == "" || strings.Contains(cleanSlug, "/") {
		return nil, ErrInvalidProductSlug
	}
	product, err := s.store.GetProductBySlug(cleanSlug)
	if err != nil {
		return nil, err
	}
	if product == nil {
		return nil, nil
	}
	mapped := mapProduct(*product)
	return &mapped, nil
}

func (s *Service) Search(query string) ([]Product, error) {
	cleanQuery := strings.TrimSpace(query)
	if cleanQuery == "" {
		return []Product{}, nil
	}
	products, err := s.store.SearchProducts(cleanQuery)
	if err != nil {
		return nil, err
	}
	return mapProducts(products), nil
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

// Package catalog serves the public product catalog.
package catalog

import (
	"errors"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/money"
	"github.com/kyambuthia/sokomoko/internal/ui"
)

var ErrInvalidProductSlug = errors.New("invalid product slug")

const (
	maxQueryLength     = 100
	defaultSearchLimit = 60
	// AllCategories is the header's "no filter" category label.
	AllCategories = "All Departments"
)

type Product struct {
	ID            int
	Name          string
	Slug          string
	Description   string
	Price         money.Cents
	StockQuantity int
	Category      string
	PartnerName   string
	ImageURL      string
	ImageAlt      string
}

// InStock reports whether at least one unit can be bought.
func (p Product) InStock() bool { return p.StockQuantity > 0 }

type Category struct {
	Name string
	Slug string
}

// Filter narrows catalog listings.
type Filter struct {
	Query    string
	Category string
	Limit    int
}

type Service struct {
	store store
}

func New(store *db.Store) *Service {
	return &Service{store: store}
}

func newWithStore(store store) *Service {
	return &Service{store: store}
}

func (s *Service) AllProducts() ([]Product, error) {
	return s.Browse(Filter{})
}

// Browse lists products, optionally filtered by a search query and category.
func (s *Service) Browse(filter Filter) ([]Product, error) {
	query := normalizeQuery(filter.Query)
	category := strings.TrimSpace(filter.Category)
	if strings.EqualFold(category, AllCategories) || strings.EqualFold(category, "all") {
		category = ""
	}
	products, err := s.store.ListProducts(db.ProductFilter{
		Query:    query,
		Category: category,
		Limit:    filter.Limit,
	})
	if err != nil {
		return nil, err
	}
	return mapProducts(products), nil
}

// Search returns products matching query; a blank query matches nothing.
func (s *Service) Search(query string) ([]Product, error) {
	return s.SearchInCategory(query, "")
}

func (s *Service) SearchInCategory(query, category string) ([]Product, error) {
	if normalizeQuery(query) == "" {
		return []Product{}, nil
	}
	return s.Browse(Filter{Query: query, Category: category, Limit: defaultSearchLimit})
}

func (s *Service) ProductBySlug(slug string) (*Product, error) {
	cleanSlug := strings.TrimSpace(slug)
	if cleanSlug == "" || strings.Contains(cleanSlug, "/") || len(cleanSlug) > 200 {
		return nil, ErrInvalidProductSlug
	}
	product, err := s.store.GetProductBySlug(cleanSlug)
	if err != nil || product == nil {
		return nil, err
	}
	mapped := mapProduct(*product)
	return &mapped, nil
}

func (s *Service) Categories() ([]Category, error) {
	categories, err := s.store.GetAllCategories()
	if err != nil {
		return nil, err
	}
	mapped := make([]Category, 0, len(categories))
	for _, c := range categories {
		mapped = append(mapped, Category{Name: c.Name, Slug: c.Slug})
	}
	return mapped, nil
}

func normalizeQuery(query string) string {
	clean := strings.Join(strings.Fields(query), " ")
	if runes := []rune(clean); len(runes) > maxQueryLength {
		clean = string(runes[:maxQueryLength])
	}
	return clean
}

func mapProducts(products []db.Product) []Product {
	mapped := make([]Product, 0, len(products))
	for _, product := range products {
		mapped = append(mapped, mapProduct(product))
	}
	return mapped
}

func mapProduct(product db.Product) Product {
	alt := product.Name
	if len(product.Images) > 0 && strings.TrimSpace(product.Images[0].AltText) != "" {
		alt = product.Images[0].AltText
	}
	return Product{
		ID:            product.ID,
		Name:          product.Name,
		Slug:          product.Slug,
		Description:   product.Description,
		Price:         product.Price,
		StockQuantity: product.StockQuantity,
		Category:      product.Category,
		PartnerName:   product.PartnerName,
		ImageURL:      ui.ProductImageURL(product.PrimaryImage(), product.Slug, product.Name, product.Category),
		ImageAlt:      alt,
	}
}

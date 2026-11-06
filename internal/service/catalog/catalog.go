// Package catalog serves the public product catalog.
package catalog

import (
	"context"
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
	maxCatalogPage     = 1_000_000
	// AllCategories is the header's "no filter" category label.
	AllCategories = "All Departments"
)

type Product struct {
	ID            int
	Name          string
	Slug          string
	Description   string
	Price         money.Cents
	Currency      string
	StockQuantity int
	Category      string
	PartnerName   string
	ImageURL      string
	ImageAlt      string
	VideoURL      string
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
	Offset   int
}

type Service struct {
	store store
}

func New(store *db.Store) *Service {
	return &Service{store: store}
}

func (s *Service) AllProducts(ctx context.Context) ([]Product, error) {
	return s.Browse(ctx, Filter{})
}

// Page returns one page of the catalog in name order and the total number of
// active products. page starts at 1.
func (s *Service) Page(ctx context.Context, page, limit int) ([]Product, int, error) {
	if page < 1 {
		page = 1
	} else if page > maxCatalogPage {
		page = maxCatalogPage
	}
	if limit < 1 {
		limit = 1
	} else if limit > 50 {
		limit = 50
	}
	products, total, err := s.store.ListProductsPage(ctx, db.ProductFilter{
		PublicOnly: true,
		Limit:      limit,
		Offset:     (page - 1) * limit,
	})
	if err != nil {
		return nil, 0, err
	}
	return mapProducts(products), total, nil
}

// Browse lists products, optionally filtered by a search query and category.
func (s *Service) Browse(ctx context.Context, filter Filter) ([]Product, error) {
	query := normalizeQuery(filter.Query)
	category := strings.TrimSpace(filter.Category)
	if strings.EqualFold(category, AllCategories) || strings.EqualFold(category, "all") {
		category = ""
	}
	products, err := s.store.ListProducts(ctx, db.ProductFilter{
		Query:      query,
		Category:   category,
		Limit:      filter.Limit,
		Offset:     filter.Offset,
		PublicOnly: true,
	})
	if err != nil {
		return nil, err
	}
	return mapProducts(products), nil
}

// Search returns products matching query; a blank query matches nothing.
func (s *Service) Search(ctx context.Context, query string) ([]Product, error) {
	return s.SearchInCategory(ctx, query, "")
}

func (s *Service) SearchInCategory(ctx context.Context, query, category string) ([]Product, error) {
	if normalizeQuery(query) == "" {
		return []Product{}, nil
	}
	return s.Browse(ctx, Filter{Query: query, Category: category, Limit: defaultSearchLimit})
}

// SearchPage returns one bounded page and the total number of catalog matches.
// The query and image fetches stop when the HTTP request is cancelled.
func (s *Service) SearchPage(ctx context.Context, query, category string, page, limit int) ([]Product, int, error) {
	query = normalizeQuery(query)
	category = strings.TrimSpace(category)
	if strings.EqualFold(category, AllCategories) || strings.EqualFold(category, "all") {
		category = ""
	}
	if query == "" && category == "" {
		return []Product{}, 0, nil
	}
	if page < 1 {
		page = 1
	} else if page > maxCatalogPage {
		page = maxCatalogPage
	}
	if limit < 1 {
		limit = 24
	} else if limit > 50 {
		limit = 50
	}

	filter := db.ProductFilter{
		Query: query, Category: category, PublicOnly: true,
		Limit: limit, Offset: (page - 1) * limit,
	}
	products, total, err := s.store.ListProductsPage(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	return mapProducts(products), total, nil
}

// ProductImage returns the bytes for an image attached to a public product.
func (s *Service) ProductImage(ctx context.Context, id int) (*db.ProductImage, error) {
	if id <= 0 {
		return nil, db.ErrProductNotFound
	}
	return s.store.GetPublicProductImageByID(ctx, id)
}

// ProductVideo returns the uploaded video for an active public product.
func (s *Service) ProductVideo(ctx context.Context, productID int) (*db.ProductVideo, error) {
	if productID <= 0 {
		return nil, db.ErrProductNotFound
	}
	return s.store.GetPublicProductVideoByProductID(ctx, productID)
}

func (s *Service) ProductBySlug(ctx context.Context, slug string) (*Product, error) {
	cleanSlug := strings.TrimSpace(slug)
	if cleanSlug == "" || strings.Contains(cleanSlug, "/") || len(cleanSlug) > 200 {
		return nil, ErrInvalidProductSlug
	}
	product, err := s.store.GetPublicProductBySlug(ctx, cleanSlug)
	if err != nil || product == nil {
		return nil, err
	}
	mapped := mapProduct(*product)
	return &mapped, nil
}

func (s *Service) Categories(ctx context.Context) ([]Category, error) {
	categories, err := s.store.GetAllCategories(ctx)
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
		Currency:      product.Currency,
		StockQuantity: product.StockQuantity,
		Category:      product.Category,
		PartnerName:   product.PartnerName,
		ImageURL:      ui.ProductImageURL(product.PrimaryImage(), product.Slug, product.Name, product.Category),
		ImageAlt:      alt,
		VideoURL:      product.VideoURL,
	}
}

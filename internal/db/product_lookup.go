package db

import (
	"database/sql"
	"log"
	"strings"
)

// GetProductBySlug retrieves a product by its slug.
func (s *Store) GetProductBySlug(slug string) (*Product, error) {
	row := s.DB.QueryRow(
		`SELECT p.id, p.name, p.slug, p.description, p.price, p.stock_quantity, c.name, p.category_id, p.created_at, p.updated_at, p.deleted_at
		 FROM products p
		 LEFT JOIN categories c ON p.category_id = c.id
		 WHERE p.slug = ? AND p.deleted_at IS NULL`,
		strings.TrimSpace(slug),
	)

	product := &Product{}
	var categoryName sql.NullString
	err := row.Scan(
		&product.ID,
		&product.Name,
		&product.Slug,
		&product.Description,
		&product.Price,
		&product.StockQuantity,
		&categoryName,
		&product.CategoryID,
		&product.CreatedAt,
		&product.UpdatedAt,
		&product.DeletedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if categoryName.Valid {
		product.Category = categoryName.String
	}

	images, err := s.GetProductImages(product.ID)
	if err != nil {
		log.Printf("Error fetching images for product %d: %v", product.ID, err)
	} else {
		product.Images = images
	}

	return product, nil
}

package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

const categoryColumns = "id, name, slug, description, parent_id, created_at, updated_at, deleted_at"

func scanCategory(row interface{ Scan(...any) error }) (*Category, error) {
	category := &Category{}
	err := row.Scan(
		&category.ID,
		&category.Name,
		&category.Slug,
		&category.Description,
		&category.ParentID,
		&category.CreatedAt,
		&category.UpdatedAt,
		&category.DeletedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return category, nil
}

func (s *Store) CreateCategory(category Category) (int64, error) {
	ctx, cancel := s.ctx()
	defer cancel()

	var id int64
	err := s.DB.QueryRowContext(ctx,
		`INSERT INTO categories (name, slug, description, parent_id)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id`,
		strings.TrimSpace(category.Name),
		strings.TrimSpace(category.Slug),
		strings.TrimSpace(category.Description),
		category.ParentID,
	).Scan(&id)
	return id, wrapDBError(err)
}

func (s *Store) GetCategoryByID(id int) (*Category, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	return scanCategory(s.DB.QueryRowContext(ctx,
		"SELECT "+categoryColumns+" FROM categories WHERE id = $1 AND deleted_at IS NULL", id,
	))
}

func (s *Store) GetCategoryBySlug(slug string) (*Category, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	return scanCategory(s.DB.QueryRowContext(ctx,
		"SELECT "+categoryColumns+" FROM categories WHERE slug = $1 AND deleted_at IS NULL",
		strings.TrimSpace(slug),
	))
}

func (s *Store) UpdateCategory(category Category) error {
	ctx, cancel := s.ctx()
	defer cancel()
	_, err := s.DB.ExecContext(ctx,
		`UPDATE categories SET name = $1, slug = $2, description = $3, parent_id = $4
		 WHERE id = $5 AND deleted_at IS NULL`,
		strings.TrimSpace(category.Name),
		strings.TrimSpace(category.Slug),
		strings.TrimSpace(category.Description),
		category.ParentID,
		category.ID,
	)
	return wrapDBError(err)
}

func (s *Store) DeleteCategory(id int) error {
	ctx, cancel := s.ctx()
	defer cancel()
	_, err := s.DB.ExecContext(ctx, "UPDATE categories SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL", id)
	return err
}

func (s *Store) GetAllCategories() ([]Category, error) {
	ctx, cancel := s.ctx()
	defer cancel()

	rows, err := s.DB.QueryContext(ctx,
		"SELECT "+categoryColumns+" FROM categories WHERE deleted_at IS NULL ORDER BY lower(name)",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	categories := []Category{}
	for rows.Next() {
		category, err := scanCategory(rows)
		if err != nil {
			return nil, err
		}
		categories = append(categories, *category)
	}
	return categories, rows.Err()
}

// productSelect projects a product with its category, partner, and available
// stock. The LATERAL sum is driven by the inventory_stocks primary key.
const productSelect = `
SELECT p.id, p.name, p.slug, p.description, p.price_cents, p.currency,
       stock.available_quantity,
       COALESCE(c.name, ''), p.category_id, p.partner_id, COALESCE(pt.name, ''),
       p.created_at, p.updated_at
FROM products p
LEFT JOIN categories c ON c.id = p.category_id AND c.deleted_at IS NULL
LEFT JOIN partners pt ON pt.id = p.partner_id
CROSS JOIN LATERAL (
    SELECT COALESCE(SUM(s.on_hand_quantity - s.reserved_quantity - s.allocated_quantity), 0)::INTEGER AS available_quantity
    FROM inventory_stocks s
    WHERE s.product_id = p.id
) stock`

func scanProduct(row interface{ Scan(...any) error }) (*Product, error) {
	product := &Product{}
	err := row.Scan(
		&product.ID,
		&product.Name,
		&product.Slug,
		&product.Description,
		&product.Price,
		&product.Currency,
		&product.StockQuantity,
		&product.Category,
		&product.CategoryID,
		&product.PartnerID,
		&product.PartnerName,
		&product.CreatedAt,
		&product.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return product, nil
}

// CreateProduct inserts a product, its opening stock in the default warehouse,
// and any images in one transaction.
func (s *Store) CreateProduct(product Product) (int64, error) {
	ctx, cancel := s.ctx()
	defer cancel()

	if product.Price < 0 {
		return 0, fmt.Errorf("price must not be negative")
	}
	if product.StockQuantity < 0 {
		return 0, ErrInvalidQuantity
	}
	currency := strings.TrimSpace(product.Currency)
	if currency == "" {
		currency = "USD"
	}

	var id int64
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		warehouseID, err := defaultWarehouseID(ctx, tx)
		if err != nil {
			return err
		}

		if err := tx.QueryRowContext(ctx,
			`INSERT INTO products (name, slug, description, price_cents, currency, category_id, partner_id)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)
			 RETURNING id`,
			strings.TrimSpace(product.Name),
			strings.TrimSpace(product.Slug),
			strings.TrimSpace(product.Description),
			int64(product.Price),
			currency,
			product.CategoryID,
			product.PartnerID,
		).Scan(&id); err != nil {
			return wrapProductCreateError(err)
		}

		if _, err := tx.ExecContext(ctx,
			`INSERT INTO inventory_stocks (product_id, warehouse_id, on_hand_quantity)
			 VALUES ($1, $2, $3)`,
			id, warehouseID, product.StockQuantity,
		); err != nil {
			return err
		}
		if product.StockQuantity > 0 {
			if err := recordStockMovement(ctx, tx, int(id), warehouseID, stockMovementInitial, product.StockQuantity, "product creation"); err != nil {
				return err
			}
		}

		for i, img := range product.Images {
			position := img.DisplayOrder
			if position <= 0 {
				position = i
			}
			if _, err := tx.ExecContext(ctx,
				"INSERT INTO product_images (product_id, url, alt_text, position) VALUES ($1, $2, $3, $4)",
				id, strings.TrimSpace(img.URL), strings.TrimSpace(img.AltText), position,
			); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (s *Store) GetProductByID(id int) (*Product, error) {
	return s.getProduct("p.id = $1", id)
}

func (s *Store) GetProductBySlug(slug string) (*Product, error) {
	return s.getProduct("p.slug = $1", strings.TrimSpace(slug))
}

func (s *Store) getProduct(predicate string, arg any) (*Product, error) {
	ctx, cancel := s.ctx()
	defer cancel()

	product, err := scanProduct(s.DB.QueryRowContext(ctx,
		productSelect+" WHERE "+predicate+" AND p.deleted_at IS NULL", arg,
	))
	if err != nil || product == nil {
		return product, err
	}

	images, err := s.getProductImagesByProductIDs(ctx, []int{product.ID})
	if err != nil {
		return nil, err
	}
	product.Images = images[product.ID]
	return product, nil
}

// UpdateProduct updates catalog fields and sets the available quantity in the
// default warehouse to product.StockQuantity, keeping reserved and allocated
// stock untouched.
func (s *Store) UpdateProduct(product Product) error {
	ctx, cancel := s.ctx()
	defer cancel()

	if product.StockQuantity < 0 {
		return ErrInvalidQuantity
	}

	return s.withTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx,
			`UPDATE products
			 SET name = $1, slug = $2, description = $3, price_cents = $4, category_id = $5
			 WHERE id = $6 AND deleted_at IS NULL`,
			strings.TrimSpace(product.Name),
			strings.TrimSpace(product.Slug),
			strings.TrimSpace(product.Description),
			int64(product.Price),
			product.CategoryID,
			product.ID,
		)
		if err != nil {
			return wrapProductCreateError(err)
		}
		affected, err := rowsAffected(result)
		if err != nil {
			return err
		}
		if affected == 0 {
			return ErrProductNotFound
		}

		warehouseID, err := defaultWarehouseID(ctx, tx)
		if err != nil {
			return err
		}
		stock, err := lockInventoryStock(ctx, tx, product.ID, warehouseID)
		if err != nil {
			return err
		}

		targetOnHand := product.StockQuantity + stock.ReservedQuantity + stock.AllocatedQuantity
		delta := targetOnHand - stock.OnHandQuantity
		if delta == 0 {
			return nil
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE inventory_stocks SET on_hand_quantity = $1
			 WHERE product_id = $2 AND warehouse_id = $3`,
			targetOnHand, product.ID, warehouseID,
		); err != nil {
			return err
		}
		return recordStockMovement(ctx, tx, product.ID, warehouseID, stockMovementAdjustment, delta, "product update")
	})
}

// DeleteProduct soft-deletes a product and removes it from carts.
func (s *Store) DeleteProduct(id int) error {
	ctx, cancel := s.ctx()
	defer cancel()

	return s.withTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "UPDATE products SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL", id); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, "DELETE FROM cart_items WHERE product_id = $1", id)
		return err
	})
}

// ProductFilter narrows ListProducts. Zero values mean "no filter".
type ProductFilter struct {
	Query       string
	Category    string // category slug or case-insensitive name
	PartnerID   int
	InStockOnly bool
	Limit       int
	Offset      int
}

// ListProducts returns active products matching the filter. A text query uses
// the full-text index and falls back to substring matching so partial words
// still match.
func (s *Store) ListProducts(filter ProductFilter) ([]Product, error) {
	ctx, cancel := s.ctx()
	defer cancel()

	var (
		where   = []string{"p.deleted_at IS NULL"}
		args    []any
		orderBy = "lower(p.name), p.id"
	)
	addArg := func(value any) string {
		args = append(args, value)
		return fmt.Sprintf("$%d", len(args))
	}

	if query := strings.TrimSpace(filter.Query); query != "" {
		tsArg := addArg(query)
		likeArg := addArg("%" + escapeLike(query) + "%")
		where = append(where, fmt.Sprintf(
			"(p.search_vector @@ websearch_to_tsquery('simple', %[1]s) OR p.name ILIKE %[2]s OR p.description ILIKE %[2]s)",
			tsArg, likeArg,
		))
		orderBy = fmt.Sprintf("ts_rank(p.search_vector, websearch_to_tsquery('simple', %s)) DESC, lower(p.name), p.id", tsArg)
	}
	if category := strings.TrimSpace(filter.Category); category != "" {
		arg := addArg(category)
		where = append(where, fmt.Sprintf("(c.slug = lower(%[1]s) OR lower(c.name) = lower(%[1]s))", arg))
	}
	if filter.PartnerID > 0 {
		where = append(where, "p.partner_id = "+addArg(filter.PartnerID))
	}
	if filter.InStockOnly {
		where = append(where, "stock.available_quantity > 0")
	}

	query := productSelect + " WHERE " + strings.Join(where, " AND ") + " ORDER BY " + orderBy
	if filter.Limit > 0 {
		query += " LIMIT " + addArg(filter.Limit)
	}
	if filter.Offset > 0 {
		query += " OFFSET " + addArg(filter.Offset)
	}

	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	products := []Product{}
	ids := []int{}
	for rows.Next() {
		product, err := scanProduct(rows)
		if err != nil {
			return nil, err
		}
		products = append(products, *product)
		ids = append(ids, product.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	images, err := s.getProductImagesByProductIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range products {
		products[i].Images = images[products[i].ID]
	}
	return products, nil
}

// GetAllProducts returns every active product ordered by name.
func (s *Store) GetAllProducts() ([]Product, error) {
	return s.ListProducts(ProductFilter{})
}

// SearchProducts matches products by name or description.
func (s *Store) SearchProducts(query string) ([]Product, error) {
	if strings.TrimSpace(query) == "" {
		return []Product{}, nil
	}
	return s.ListProducts(ProductFilter{Query: query})
}

func (s *Store) CountProducts() (int, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	var count int
	err := s.DB.QueryRowContext(ctx, "SELECT count(*) FROM products WHERE deleted_at IS NULL").Scan(&count)
	return count, err
}

func (s *Store) CreateProductImage(img ProductImage) (int64, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	var id int64
	err := s.DB.QueryRowContext(ctx,
		"INSERT INTO product_images (product_id, url, alt_text, position) VALUES ($1, $2, $3, $4) RETURNING id",
		img.ProductID, strings.TrimSpace(img.URL), strings.TrimSpace(img.AltText), img.DisplayOrder,
	).Scan(&id)
	return id, wrapDBError(err)
}

func (s *Store) GetProductImages(productID int) ([]ProductImage, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	images, err := s.getProductImagesByProductIDs(ctx, []int{productID})
	if err != nil {
		return nil, err
	}
	return images[productID], nil
}

func (s *Store) GetProductImagesByProductIDs(productIDs []int) (map[int][]ProductImage, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	return s.getProductImagesByProductIDs(ctx, productIDs)
}

func (s *Store) getProductImagesByProductIDs(ctx context.Context, productIDs []int) (map[int][]ProductImage, error) {
	imagesByProduct := map[int][]ProductImage{}
	if len(productIDs) == 0 {
		return imagesByProduct, nil
	}

	rows, err := s.DB.QueryContext(ctx,
		`SELECT id, product_id, url, alt_text, position, created_at
		 FROM product_images
		 WHERE product_id = ANY($1)
		 ORDER BY product_id, position, id`,
		int64Slice(productIDs),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		img := ProductImage{}
		if err := rows.Scan(&img.ID, &img.ProductID, &img.URL, &img.AltText, &img.DisplayOrder, &img.CreatedAt); err != nil {
			return nil, err
		}
		imagesByProduct[img.ProductID] = append(imagesByProduct[img.ProductID], img)
	}
	return imagesByProduct, rows.Err()
}

func (s *Store) DeleteProductImage(id int) error {
	ctx, cancel := s.ctx()
	defer cancel()
	_, err := s.DB.ExecContext(ctx, "DELETE FROM product_images WHERE id = $1", id)
	return err
}

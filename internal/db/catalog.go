package db

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
)

// CreateCategory inserts a new category into the database
func (s *Store) CreateCategory(category Category) (int64, error) {
	stmt, err := s.DB.Prepare(
		"INSERT INTO categories (name, slug, description, parent_id) VALUES (?, ?, ?, ?)")
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	res, err := stmt.Exec(category.Name, category.Slug, category.Description, category.ParentID)
	if err != nil {
		return 0, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return id, nil
}

// GetCategoryByID retrieves a category by its ID
func (s *Store) GetCategoryByID(id int) (*Category, error) {
	row := s.DB.QueryRow(
		"SELECT id, name, slug, description, parent_id, created_at, updated_at, deleted_at FROM categories WHERE id = ? AND deleted_at IS NULL", id)

	category := &Category{}
	err := row.Scan(
		&category.ID,
		&category.Name,
		&category.Slug,
		&category.Description,
		&category.ParentID,
		&category.CreatedAt,
		&category.UpdatedAt,
		&category.DeletedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return category, nil
}

// UpdateCategory updates an existing category's information
func (s *Store) UpdateCategory(category Category) error {
	stmt, err := s.DB.Prepare(
		"UPDATE categories SET name = ?, slug = ?, description = ?, parent_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(category.Name, category.Slug, category.Description, category.ParentID, category.ID)
	if err != nil {
		return err
	}
	return nil
}

// DeleteCategory deletes a category from the database by ID
func (s *Store) DeleteCategory(id int) error {
	stmt, err := s.DB.Prepare("UPDATE categories SET deleted_at = CURRENT_TIMESTAMP WHERE id = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(id)
	if err != nil {
		return err
	}
	return nil
}

func (s *Store) GetAllCategories() ([]Category, error) {
	rows, err := s.DB.Query(
		"SELECT id, name, slug, description, parent_id, created_at, updated_at, deleted_at FROM categories WHERE deleted_at IS NULL ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	categories := []Category{}
	for rows.Next() {
		category := Category{}
		if err := rows.Scan(
			&category.ID,
			&category.Name,
			&category.Slug,
			&category.Description,
			&category.ParentID,
			&category.CreatedAt,
			&category.UpdatedAt,
			&category.DeletedAt,
		); err != nil {
			return nil, err
		}
		categories = append(categories, category)
	}
	return categories, nil
}

// CreateProduct inserts a new product into the database
func (s *Store) CreateProduct(product Product) (int64, error) {
	stmt, err := s.DB.Prepare(
		"INSERT INTO products (name, slug, description, price, stock_quantity, category_id) VALUES (?, ?, ?, ?, ?, ?)")
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	res, err := stmt.Exec(product.Name, product.Slug, product.Description, product.Price, product.StockQuantity, product.CategoryID)
	if err != nil {
		return 0, wrapProductCreateError(err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	for _, img := range product.Images {
		img.ProductID = int(id)
		_, _ = s.CreateProductImage(img)
	}

	return id, nil
}

// GetProductByID retrieves a product by its ID
func (s *Store) GetProductByID(id int) (*Product, error) {
	row := s.DB.QueryRow(
		`SELECT p.id, p.name, p.slug, p.description, p.price, p.stock_quantity, c.name, p.category_id, p.created_at, p.updated_at, p.deleted_at
		 FROM products p
		 LEFT JOIN categories c ON p.category_id = c.id
		 WHERE p.id = ? AND p.deleted_at IS NULL`, id)

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
		&product.DeletedAt)

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

// UpdateProduct updates an existing product's information
func (s *Store) UpdateProduct(product Product) error {
	stmt, err := s.DB.Prepare(
		"UPDATE products SET name = ?, slug = ?, description = ?, price = ?, stock_quantity = ?, category_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(product.Name, product.Slug, product.Description, product.Price, product.StockQuantity, product.CategoryID, product.ID)
	if err != nil {
		return err
	}
	return nil
}

// DeleteProduct deletes a product from the database by ID
func (s *Store) DeleteProduct(id int) error {
	stmt, err := s.DB.Prepare("UPDATE products SET deleted_at = CURRENT_TIMESTAMP WHERE id = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(id)
	if err != nil {
		return err
	}
	return nil
}

// SearchProducts searches for products by name or description
func (s *Store) SearchProducts(query string) ([]Product, error) {
	if query == "" {
		return []Product{}, nil
	}

	rows, err := s.DB.Query(
		`SELECT p.id, p.name, p.slug, p.description, p.price, p.stock_quantity, c.name, p.category_id, p.created_at, p.updated_at, p.deleted_at
		 FROM products p
		 LEFT JOIN categories c ON p.category_id = c.id
		 WHERE (p.name LIKE ? OR p.description LIKE ?) AND p.deleted_at IS NULL
		 ORDER BY p.name`,
		"%"+query+"%", "%"+query+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var products []Product
	productIDs := []int{}
	for rows.Next() {
		product := Product{}
		var categoryName sql.NullString
		err := rows.Scan(
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
			&product.DeletedAt)
		if err != nil {
			return nil, err
		}

		if categoryName.Valid {
			product.Category = categoryName.String
		}

		products = append(products, product)
		productIDs = append(productIDs, product.ID)
	}
	imagesByProduct, err := s.GetProductImagesByProductIDs(productIDs)
	if err != nil {
		return nil, err
	}
	for i := range products {
		products[i].Images = imagesByProduct[products[i].ID]
	}
	return products, nil
}

// GetAllProducts retrieves all products from the database
func (s *Store) GetAllProducts() ([]Product, error) {
	rows, err := s.DB.Query(
		`SELECT p.id, p.name, p.slug, p.description, p.price, p.stock_quantity, c.name, p.category_id, p.created_at, p.updated_at, p.deleted_at
		 FROM products p
		 LEFT JOIN categories c ON p.category_id = c.id
		 WHERE p.deleted_at IS NULL
		 ORDER BY p.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var products []Product
	productIDs := []int{}
	for rows.Next() {
		product := Product{}
		var categoryName sql.NullString
		err := rows.Scan(
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
			&product.DeletedAt)
		if err != nil {
			return nil, err
		}

		if categoryName.Valid {
			product.Category = categoryName.String
		}

		products = append(products, product)
		productIDs = append(productIDs, product.ID)
	}
	imagesByProduct, err := s.GetProductImagesByProductIDs(productIDs)
	if err != nil {
		return nil, err
	}
	for i := range products {
		products[i].Images = imagesByProduct[products[i].ID]
	}
	return products, nil
}

func (s *Store) CreateProductImage(img ProductImage) (int64, error) {
	stmt, err := s.DB.Prepare("INSERT INTO product_images (product_id, url, alt_text, display_order) VALUES (?, ?, ?, ?)")
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	res, err := stmt.Exec(img.ProductID, img.URL, img.AltText, img.DisplayOrder)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) GetProductImages(productID int) ([]ProductImage, error) {
	rows, err := s.DB.Query("SELECT id, product_id, url, alt_text, display_order, created_at FROM product_images WHERE product_id = ? ORDER BY display_order", productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var images []ProductImage
	for rows.Next() {
		img := ProductImage{}
		err := rows.Scan(&img.ID, &img.ProductID, &img.URL, &img.AltText, &img.DisplayOrder, &img.CreatedAt)
		if err != nil {
			return nil, err
		}
		images = append(images, img)
	}
	return images, nil
}

func (s *Store) GetProductImagesByProductIDs(productIDs []int) (map[int][]ProductImage, error) {
	imagesByProduct := map[int][]ProductImage{}
	if len(productIDs) == 0 {
		return imagesByProduct, nil
	}

	placeholders := make([]string, len(productIDs))
	args := make([]interface{}, 0, len(productIDs))
	for i, id := range productIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}

	query := fmt.Sprintf(
		`SELECT id, product_id, url, alt_text, display_order, created_at
		 FROM product_images
		 WHERE product_id IN (%s)
		 ORDER BY product_id, display_order`,
		strings.Join(placeholders, ","),
	)
	rows, err := s.DB.Query(query, args...)
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
	return imagesByProduct, nil
}

func (s *Store) DeleteProductImage(id int) error {
	_, err := s.DB.Exec("DELETE FROM product_images WHERE id = ?", id)
	return err
}

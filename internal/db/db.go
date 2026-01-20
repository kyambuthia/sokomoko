package db

import (
	"database/sql"
	"log"
	"os"
	"time"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
)

var DB *sql.DB

const SchemaSQL = `
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    salt TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'user', -- 'user', 'admin'
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS categories (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    description TEXT,
    parent_id INTEGER, -- For hierarchical categories
    FOREIGN KEY (parent_id) REFERENCES categories(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS products (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    description TEXT,
    price REAL NOT NULL,
    stock_quantity INTEGER NOT NULL DEFAULT 0,
    category_id INTEGER,
    image_url TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (category_id) REFERENCES categories(id) ON DELETE SET NULL
);
`

func InitDB() {
	var err error
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "./db/t.db"
	}
	DB, err = sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatal(err)
	}

	if err = DB.Ping(); err != nil {
		log.Fatal(err)
	}

	// Apply schema
	_, err = DB.Exec(SchemaSQL)
	if err != nil {
		log.Fatal(err)
	}
}

// User represents a user in the system
type User struct {
	ID           int
	Username     string
	Email        string
	PasswordHash string
	Salt         string
	Role         string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Category represents a product category
type Category struct {
	ID          int
	Name        string
	Description string
	ParentID    sql.NullInt64 // Use sql.NullInt64 for nullable integers
}

// Product represents a product in the system
type Product struct {
	ID            int
	Name          string
	Description   string
	Price         float64
	StockQuantity int
	Category      string
	ImageURL      string // Not in database, kept for frontend compatibility
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// CreateUser inserts a new user into the database
func CreateUser(user User) (int64, error) {
	stmt, err := DB.Prepare(
		"INSERT INTO users (username, email, password_hash, salt, role) VALUES (?, ?, ?, ?, ?)")
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	res, err := stmt.Exec(user.Username, user.Email, user.PasswordHash, user.Salt, user.Role)
	if err != nil {
		return 0, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return id, nil
}

// GetUserByUsername retrieves a user by their username
func GetUserByUsername(username string) (*User, error) {
	row := DB.QueryRow(
		"SELECT id, username, email, password_hash, salt, role, created_at, updated_at FROM users WHERE username = ?", username)

	user := &User{}
	err := row.Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.PasswordHash,
		&user.Salt,
		&user.Role,
		&user.CreatedAt,
		&user.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil // User not found
	}
	if err != nil {
		return nil, err
	}
	return user, nil
}

// GetUserByID retrieves a user by their ID
func GetUserByID(id int) (*User, error) {
	row := DB.QueryRow(
		"SELECT id, username, email, password_hash, salt, role, created_at, updated_at FROM users WHERE id = ?", id)

	user := &User{}
	err := row.Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.PasswordHash,
		&user.Salt,
		&user.Role,
		&user.CreatedAt,
		&user.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil // User not found
	}
	if err != nil {
		return nil, err
	}
	return user, nil
}

// UpdateUser updates an existing user's information
func UpdateUser(user User) error {
	stmt, err := DB.Prepare(
		"UPDATE users SET username = ?, email = ?, password_hash = ?, salt = ?, role = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(user.Username, user.Email, user.PasswordHash, user.Salt, user.Role, user.ID)
	if err != nil {
		return err
	}
	return nil
}

// DeleteUser deletes a user from the database by ID
func DeleteUser(id int) error {
	stmt, err := DB.Prepare("DELETE FROM users WHERE id = ?")
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

// CreateCategory inserts a new category into the database
func CreateCategory(category Category) (int64, error) {
	stmt, err := DB.Prepare(
		"INSERT INTO categories (name, description, parent_id) VALUES (?, ?, ?)")
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	res, err := stmt.Exec(category.Name, category.Description, category.ParentID)
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
func GetCategoryByID(id int) (*Category, error) {
	row := DB.QueryRow(
		"SELECT id, name, description, parent_id FROM categories WHERE id = ?", id)

	category := &Category{}
	err := row.Scan(
		&category.ID,
		&category.Name,
		&category.Description,
		&category.ParentID)

	if err == sql.ErrNoRows {
		return nil, nil // Category not found
	}
	if err != nil {
		return nil, err
	}
	return category, nil
}

// UpdateCategory updates an existing category's information
func UpdateCategory(category Category) error {
	stmt, err := DB.Prepare(
		"UPDATE categories SET name = ?, description = ?, parent_id = ? WHERE id = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(category.Name, category.Description, category.ParentID, category.ID)
	if err != nil {
		return err
	}
	return nil
}

// DeleteCategory deletes a category from the database by ID
func DeleteCategory(id int) error {
	stmt, err := DB.Prepare("DELETE FROM categories WHERE id = ?")
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

// CreateProduct inserts a new product into the database
func CreateProduct(product Product) (int64, error) {
	stmt, err := DB.Prepare(
		"INSERT INTO products (name, description, price, stock_quantity, category) VALUES (?, ?, ?, ?, ?)")
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	res, err := stmt.Exec(product.Name, product.Description, product.Price, product.StockQuantity, "general")
	if err != nil {
		return 0, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return id, nil
}

// GetProductByID retrieves a product by its ID
func GetProductByID(id int) (*Product, error) {
	row := DB.QueryRow(
		"SELECT id, name, description, price, stock_quantity, category, created_at, updated_at FROM products WHERE id = ?", id)

	product := &Product{}
	err := row.Scan(
		&product.ID,
		&product.Name,
		&product.Description,
		&product.Price,
		&product.StockQuantity,
		&product.Category,
		&product.CreatedAt,
		&product.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil // Product not found
	}
	if err != nil {
		return nil, err
	}
	return product, nil
}

// UpdateProduct updates an existing product's information
func UpdateProduct(product Product) error {
	stmt, err := DB.Prepare(
		"UPDATE products SET name = ?, description = ?, price = ?, stock_quantity = ?, category = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(product.Name, product.Description, product.Price, product.StockQuantity, product.Category, product.ID)
	if err != nil {
		return err
	}
	return nil
}

// DeleteProduct deletes a product from the database by ID
func DeleteProduct(id int) error {
	stmt, err := DB.Prepare("DELETE FROM products WHERE id = ?")
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
func SearchProducts(query string) ([]Product, error) {
	if query == "" {
		return []Product{}, nil
	}

	// Use LIKE to search for products where name or description contains the query
	rows, err := DB.Query(
		"SELECT id, name, description, price, stock_quantity, category, created_at, updated_at FROM products WHERE name LIKE ? OR description LIKE ? ORDER BY name",
		"%"+query+"%", "%"+query+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var products []Product
	for rows.Next() {
		product := &Product{}
		var createdAt, updatedAt sql.NullTime
		err := rows.Scan(
			&product.ID,
			&product.Name,
			&product.Description,
			&product.Price,
			&product.StockQuantity,
			&product.Category,
			&createdAt,
			&updatedAt)
		if err != nil {
			return nil, err
		}

		// Handle null timestamps
		if createdAt.Valid {
			product.CreatedAt = createdAt.Time
		} else {
			product.CreatedAt = time.Now()
		}
		if updatedAt.Valid {
			product.UpdatedAt = updatedAt.Time
		} else {
			product.UpdatedAt = time.Now()
		}

		products = append(products, *product)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return products, nil
}

// GetAllProducts retrieves all products from the database
func GetAllProducts() ([]Product, error) {
	rows, err := DB.Query(
		"SELECT id, name, description, price, stock_quantity, category, created_at, updated_at FROM products ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var products []Product
	for rows.Next() {
		product := &Product{}
		var createdAt, updatedAt sql.NullTime
		err := rows.Scan(
			&product.ID,
			&product.Name,
			&product.Description,
			&product.Price,
			&product.StockQuantity,
			&product.Category,
			&createdAt,
			&updatedAt)
		if err != nil {
			return nil, err
		}

		// Handle null timestamps
		if createdAt.Valid {
			product.CreatedAt = createdAt.Time
		} else {
			product.CreatedAt = time.Now()
		}
		if updatedAt.Valid {
			product.UpdatedAt = updatedAt.Time
		} else {
			product.UpdatedAt = time.Now()
		}

		products = append(products, *product)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return products, nil
}

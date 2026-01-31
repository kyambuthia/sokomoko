package db

import (
	"database/sql"
	"log"
	"os"
	"time"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
	"golang.org/x/crypto/bcrypt"
)

type Store struct {
	DB *sql.DB
}

const SchemaSQL = `
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    salt TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'user',
    slug TEXT UNIQUE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    deleted_at DATETIME
);

CREATE TABLE IF NOT EXISTS categories (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    slug TEXT NOT NULL UNIQUE,
    description TEXT,
    parent_id INTEGER,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    deleted_at DATETIME,
    FOREIGN KEY (parent_id) REFERENCES categories(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS products (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    description TEXT,
    price REAL NOT NULL,
    stock_quantity INTEGER NOT NULL DEFAULT 0,
    category_id INTEGER,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    deleted_at DATETIME,
    FOREIGN KEY (category_id) REFERENCES categories(id) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS product_images (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    product_id INTEGER NOT NULL,
    url TEXT NOT NULL,
    alt_text TEXT,
    display_order INTEGER DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    user_id INTEGER NOT NULL,
    expires_at DATETIME NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
`

func InitDB() {
	_, err := OpenStoreFromEnv()
	if err != nil {
		log.Fatal(err)
	}
}

func OpenStoreFromEnv() (*Store, error) {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "./db/t.db"
	}
	return OpenStore(dbPath)
}

func OpenStore(dbPath string) (*Store, error) {
	store, err := openStoreWithSchema(dbPath)
	if err != nil {
		return nil, err
	}
	store.SeedAdmin()
	return store, nil
}

func OpenStoreNoSeed(dbPath string) (*Store, error) {
	return openStoreWithSchema(dbPath)
}

func openStoreWithSchema(dbPath string) (*Store, error) {
	dbConn, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	if err = dbConn.Ping(); err != nil {
		_ = dbConn.Close()
		return nil, err
	}

	_, err = dbConn.Exec(SchemaSQL)
	if err != nil {
		_ = dbConn.Close()
		return nil, err
	}

	return &Store{DB: dbConn}, nil
}

func (s *Store) Close() error {
	if s == nil || s.DB == nil {
		return nil
	}
	return s.DB.Close()
}

func (s *Store) SeedAdmin() {
	var count int
	err := s.DB.QueryRow("SELECT COUNT(*) FROM users WHERE role = 'admin'").Scan(&count)
	if err != nil {
		log.Printf("Error checking for admin user: %v", err)
		return
	}

	if count == 0 {
		password := "adminpass"
		// Use a simple salt for seeding
		salt := "static_seed_salt"
		hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(password+salt), bcrypt.DefaultCost)

		user := User{
			Username:     "admin",
			Email:        "admin@sokomoko.com",
			PasswordHash: string(hashedPassword),
			Salt:         salt,
			Role:         "admin",
			Slug:         "admin",
		}

		_, err = s.CreateUser(user)
		if err != nil {
			log.Printf("Error seeding admin user: %v", err)
		} else {
			log.Println("Default admin user created: admin / adminpass")
		}
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
	Slug         string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    sql.NullTime
}

// Category represents a product category
type Category struct {
	ID          int
	Name        string
	Slug        string
	Description string
	ParentID    sql.NullInt64
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   sql.NullTime
}

// Product represents a product in the system
type Product struct {
	ID            int
	Name          string
	Slug          string
	Description   string
	Price         float64
	StockQuantity int
	Category      string
	CategoryID    sql.NullInt64
	Images        []ProductImage
	CreatedAt     time.Time
	UpdatedAt     time.Time
	DeletedAt     sql.NullTime
}

// GetPrimaryImageURL returns the first image URL or a placeholder
func (p Product) GetPrimaryImageURL() string {
	if len(p.Images) > 0 {
		return p.Images[0].URL
	}
	return "/static/images/placeholder.png"
}

// ProductImage represents an image for a product
type ProductImage struct {
	ID           int
	ProductID    int
	URL          string
	AltText      string
	DisplayOrder int
	CreatedAt    time.Time
}

// Session represents a persistent user session
type Session struct {
	ID        string
	UserID    int
	ExpiresAt time.Time
	CreatedAt time.Time
}

// CreateUser inserts a new user into the database
func (s *Store) CreateUser(user User) (int64, error) {
	if user.Slug == "" {
		user.Slug = user.Username
	}

	stmt, err := s.DB.Prepare(
		"INSERT INTO users (username, email, password_hash, salt, role, slug) VALUES (?, ?, ?, ?, ?, ?)")
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	res, err := stmt.Exec(user.Username, user.Email, user.PasswordHash, user.Salt, user.Role, user.Slug)
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
func (s *Store) GetUserByUsername(username string) (*User, error) {
	row := s.DB.QueryRow(
		"SELECT id, username, email, password_hash, salt, role, slug, created_at, updated_at, deleted_at FROM users WHERE username = ? AND deleted_at IS NULL", username)

	user := &User{}
	err := row.Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.PasswordHash,
		&user.Salt,
		&user.Role,
		&user.Slug,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.DeletedAt)

	if err == sql.ErrNoRows {
		return nil, nil // User not found
	}
	if err != nil {
		return nil, err
	}
	return user, nil
}

// GetUserByID retrieves a user by their ID
func (s *Store) GetUserByID(id int) (*User, error) {
	row := s.DB.QueryRow(
		"SELECT id, username, email, password_hash, salt, role, slug, created_at, updated_at, deleted_at FROM users WHERE id = ? AND deleted_at IS NULL", id)

	user := &User{}
	err := row.Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.PasswordHash,
		&user.Salt,
		&user.Role,
		&user.Slug,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.DeletedAt)

	if err == sql.ErrNoRows {
		return nil, nil // User not found
	}
	if err != nil {
		return nil, err
	}
	return user, nil
}

// UpdateUser updates an existing user's information
func (s *Store) UpdateUser(user User) error {
	stmt, err := s.DB.Prepare(
		"UPDATE users SET username = ?, email = ?, password_hash = ?, salt = ?, role = ?, slug = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(user.Username, user.Email, user.PasswordHash, user.Salt, user.Role, user.Slug, user.ID)
	if err != nil {
		return err
	}
	return nil
}

// DeleteUser deletes a user from the database by ID
func (s *Store) DeleteUser(id int) error {
	stmt, err := s.DB.Prepare("UPDATE users SET deleted_at = CURRENT_TIMESTAMP WHERE id = ?")
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
		return nil, nil // Category not found
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
		return 0, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	// Create images if any
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
		return nil, nil // Product not found
	}
	if err != nil {
		return nil, err
	}

	if categoryName.Valid {
		product.Category = categoryName.String
	}

	// Fetch images
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

		// Fetch images
		imgs, _ := s.GetProductImages(product.ID)
		product.Images = imgs

		products = append(products, product)
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

		// Fetch images
		imgs, _ := s.GetProductImages(product.ID)
		product.Images = imgs

		products = append(products, product)
	}
	return products, nil
}

// --- Image Management ---

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

func (s *Store) DeleteProductImage(id int) error {
	_, err := s.DB.Exec("DELETE FROM product_images WHERE id = ?", id)
	return err
}

// --- Session Management ---

func (s *Store) CreateSession(sess Session) error {
	stmt, err := s.DB.Prepare("INSERT INTO sessions (id, user_id, expires_at) VALUES (?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(sess.ID, sess.UserID, sess.ExpiresAt)
	return err
}

func (s *Store) GetSession(id string) (*Session, error) {
	row := s.DB.QueryRow("SELECT id, user_id, expires_at, created_at FROM sessions WHERE id = ?", id)
	sess := &Session{}
	err := row.Scan(&sess.ID, &sess.UserID, &sess.ExpiresAt, &sess.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return sess, nil
}

func (s *Store) DeleteSession(id string) error {
	_, err := s.DB.Exec("DELETE FROM sessions WHERE id = ?", id)
	return err
}

func (s *Store) CleanupSessions() error {
	_, err := s.DB.Exec("DELETE FROM sessions WHERE expires_at < CURRENT_TIMESTAMP")
	return err
}

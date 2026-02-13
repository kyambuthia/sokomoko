package db

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strings"
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
    role TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('user', 'staff', 'admin')),
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
    price REAL NOT NULL CHECK (price >= 0),
    stock_quantity INTEGER NOT NULL DEFAULT 0 CHECK (stock_quantity >= 0),
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

CREATE TABLE IF NOT EXISTS password_reset_tokens (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    token_hash TEXT NOT NULL UNIQUE,
    user_id INTEGER NOT NULL,
    expires_at DATETIME NOT NULL,
    used_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS store_settings (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    store_name TEXT NOT NULL,
    store_slug TEXT NOT NULL UNIQUE,
    description TEXT,
    contact_email TEXT NOT NULL,
    initialized_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS carts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL UNIQUE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS cart_items (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    cart_id INTEGER NOT NULL,
    product_id INTEGER NOT NULL,
    quantity INTEGER NOT NULL CHECK (quantity > 0),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (cart_id, product_id),
    FOREIGN KEY (cart_id) REFERENCES carts(id) ON DELETE CASCADE,
    FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS orders (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'shipped', 'delivered', 'cancelled')),
    partner_status TEXT NOT NULL DEFAULT 'new' CHECK (partner_status IN ('new', 'accepted', 'packing', 'dispatched', 'completed', 'cancelled')),
    delivery_status TEXT NOT NULL DEFAULT 'queued' CHECK (delivery_status IN ('queued', 'processing', 'shipped', 'delivered')),
    total_amount REAL NOT NULL CHECK (total_amount >= 0),
    delivery_address TEXT NOT NULL,
    delivery_notice TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS order_items (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    order_id INTEGER NOT NULL,
    product_id INTEGER NOT NULL,
    product_name TEXT NOT NULL,
    quantity INTEGER NOT NULL CHECK (quantity > 0),
    unit_price REAL NOT NULL CHECK (unit_price >= 0),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (order_id) REFERENCES orders(id) ON DELETE CASCADE,
    FOREIGN KEY (product_id) REFERENCES products(id)
);

CREATE TABLE IF NOT EXISTS audit_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    actor_user_id INTEGER,
    action TEXT NOT NULL,
    target_type TEXT NOT NULL,
    target_id INTEGER,
    details TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (actor_user_id) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_users_deleted_at ON users(deleted_at);
CREATE INDEX IF NOT EXISTS idx_products_deleted_at ON products(deleted_at);
CREATE INDEX IF NOT EXISTS idx_product_images_product_id ON product_images(product_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);
CREATE INDEX IF NOT EXISTS idx_password_reset_user_id ON password_reset_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_password_reset_expires_at ON password_reset_tokens(expires_at);
CREATE INDEX IF NOT EXISTS idx_store_settings_slug ON store_settings(store_slug);
CREATE INDEX IF NOT EXISTS idx_carts_user_id ON carts(user_id);
CREATE INDEX IF NOT EXISTS idx_cart_items_cart_id ON cart_items(cart_id);
CREATE INDEX IF NOT EXISTS idx_orders_user_id ON orders(user_id);
CREATE INDEX IF NOT EXISTS idx_orders_partner_status ON orders(partner_status);
CREATE INDEX IF NOT EXISTS idx_order_items_order_id ON order_items(order_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at ON audit_logs(created_at);
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
	store.SeedInitialCatalog()
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

	if _, err = dbConn.Exec("PRAGMA foreign_keys = ON; PRAGMA busy_timeout = 5000;"); err != nil {
		_ = dbConn.Close()
		return nil, err
	}

	// Keep pool bounded but allow nested read queries used by rendering paths.
	dbConn.SetMaxOpenConns(10)
	dbConn.SetMaxIdleConns(10)
	dbConn.SetConnMaxLifetime(0)

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
		password := strings.TrimSpace(os.Getenv("ADMIN_PASSWORD"))
		if password == "" {
			log.Printf("No ADMIN_PASSWORD configured; skipping admin bootstrap user seeding")
			return
		}

		salt, err := generateSalt()
		if err != nil {
			log.Printf("Error generating admin salt: %v", err)
			return
		}

		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password+salt), bcrypt.DefaultCost)
		if err != nil {
			log.Printf("Error hashing admin password: %v", err)
			return
		}

		adminUsername := strings.TrimSpace(os.Getenv("ADMIN_USERNAME"))
		if adminUsername == "" {
			adminUsername = "admin"
		}

		adminEmail := strings.TrimSpace(os.Getenv("ADMIN_EMAIL"))
		if adminEmail == "" {
			adminEmail = "admin@sokomoko.com"
		}

		user := User{
			Username:     adminUsername,
			Email:        adminEmail,
			PasswordHash: string(hashedPassword),
			Salt:         salt,
			Role:         "admin",
			Slug:         adminUsername,
		}

		_, err = s.CreateUser(user)
		if err != nil {
			log.Printf("Error seeding admin user: %v", err)
		} else {
			log.Printf("Admin bootstrap user created: %s", adminUsername)
		}
	}
}

func (s *Store) SeedInitialCatalog() {
	if s == nil || s.DB == nil {
		return
	}

	catalog := []struct {
		Name         string
		Slug         string
		Description  string
		Price        float64
		Stock        int
		Category     string
		CategorySlug string
		ImageURL     string
	}{
		{
			Name:         "Wireless Earbuds Charging Case",
			Slug:         "wireless-earbuds-charging-case",
			Description:  "Compact wireless earbuds with charging case and balanced audio profile.",
			Price:        89.99,
			Stock:        42,
			Category:     "Electronics",
			CategorySlug: "electronics",
			ImageURL:     "/static/images/white_wireless_earbuds_charging_case.png",
		},
		{
			Name:         "Over-Ear Headphones",
			Slug:         "over-ear-headphones",
			Description:  "Comfortable over-ear headphones for everyday listening and calls.",
			Price:        129.00,
			Stock:        33,
			Category:     "Electronics",
			CategorySlug: "electronics",
			ImageURL:     "/static/images/black_over_ear_headphones.png",
		},
		{
			Name:         "Portable Bluetooth Speaker",
			Slug:         "portable-bluetooth-speaker",
			Description:  "Fabric-finish wireless speaker with clear mids and portable form factor.",
			Price:        74.50,
			Stock:        29,
			Category:     "Electronics",
			CategorySlug: "electronics",
			ImageURL:     "/static/images/gray_fabric_bluetooth_speaker.png",
		},
		{
			Name:         "Electric Kettle",
			Slug:         "electric-kettle",
			Description:  "Stainless steel electric kettle suitable for daily tea and coffee prep.",
			Price:        49.99,
			Stock:        26,
			Category:     "Home & Kitchen",
			CategorySlug: "home-kitchen",
			ImageURL:     "/static/images/stainless_steel_electric_kettle.png",
		},
		{
			Name:         "Adjustable Desk Lamp",
			Slug:         "adjustable-desk-lamp",
			Description:  "Adjustable desk lamp for focused workspace lighting.",
			Price:        39.95,
			Stock:        37,
			Category:     "Home & Office",
			CategorySlug: "home-office",
			ImageURL:     "/static/images/black_adjustable_desk_lamp.png",
		},
	}

	for _, item := range catalog {
		categoryID, err := s.ensureCategory(item.Category, item.CategorySlug)
		if err != nil {
			log.Printf("Skipping seed product %s: failed to ensure category: %v", item.Slug, err)
			continue
		}

		productID, existed, err := s.ensureProduct(item, categoryID)
		if err != nil {
			log.Printf("Skipping seed product %s: %v", item.Slug, err)
			continue
		}

		if err := s.ensurePrimaryImage(productID, item.ImageURL); err != nil {
			log.Printf("Seed product image warning for %s: %v", item.Slug, err)
			continue
		}

		if !existed {
			log.Printf("Seeded product: %s", item.Name)
		}
	}
}

func (s *Store) ensureCategory(name, slug string) (int64, error) {
	var id int64
	err := s.DB.QueryRow("SELECT id FROM categories WHERE slug = ? AND deleted_at IS NULL", slug).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}

	newID, err := s.CreateCategory(Category{
		Name:        name,
		Slug:        slug,
		Description: name + " products",
	})
	if err != nil {
		var existingID int64
		lookupErr := s.DB.QueryRow("SELECT id FROM categories WHERE slug = ? AND deleted_at IS NULL", slug).Scan(&existingID)
		if lookupErr == nil {
			return existingID, nil
		}
		return 0, err
	}
	return newID, nil
}

func (s *Store) ensureProduct(item struct {
	Name         string
	Slug         string
	Description  string
	Price        float64
	Stock        int
	Category     string
	CategorySlug string
	ImageURL     string
}, categoryID int64) (int, bool, error) {
	var id int
	err := s.DB.QueryRow("SELECT id FROM products WHERE slug = ? AND deleted_at IS NULL", item.Slug).Scan(&id)
	if err == nil {
		return id, true, nil
	}
	if err != sql.ErrNoRows {
		return 0, false, err
	}

	newID, err := s.CreateProduct(Product{
		Name:          item.Name,
		Slug:          item.Slug,
		Description:   item.Description,
		Price:         item.Price,
		StockQuantity: item.Stock,
		CategoryID:    sql.NullInt64{Int64: categoryID, Valid: true},
	})
	if err != nil {
		return 0, false, err
	}
	return int(newID), false, nil
}

func (s *Store) ensurePrimaryImage(productID int, imageURL string) error {
	var existingCount int
	err := s.DB.QueryRow(
		"SELECT COUNT(*) FROM product_images WHERE product_id = ? AND url = ?",
		productID, imageURL,
	).Scan(&existingCount)
	if err != nil {
		return err
	}
	if existingCount > 0 {
		return nil
	}

	_, err = s.CreateProductImage(ProductImage{
		ProductID:    productID,
		URL:          imageURL,
		AltText:      "Product image",
		DisplayOrder: 0,
	})
	return err
}

func generateSalt() (string, error) {
	saltBytes := make([]byte, 16)
	if _, err := rand.Read(saltBytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(saltBytes), nil
}

func hashSessionID(token string) string {
	return hashOpaqueToken(token)
}

func hashOpaqueToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
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

type PasswordResetToken struct {
	ID        int
	TokenHash string
	UserID    int
	ExpiresAt time.Time
	UsedAt    sql.NullTime
	CreatedAt time.Time
}

type StoreSettings struct {
	ID            int
	StoreName     string
	StoreSlug     string
	Description   string
	ContactEmail  string
	InitializedAt time.Time
	UpdatedAt     time.Time
}

type CartItem struct {
	ProductID       int
	ProductName     string
	ProductSlug     string
	ProductImageURL string
	UnitPrice       float64
	Quantity        int
	StockQuantity   int
	LineTotal       float64
}

type OrderItem struct {
	ProductID   int
	ProductName string
	Quantity    int
	UnitPrice   float64
	LineTotal   float64
}

type CustomerOrder struct {
	ID              int
	Status          string
	PartnerStatus   string
	DeliveryStatus  string
	DeliveryAddress string
	DeliveryNotice  string
	TotalAmount     float64
	CreatedAt       time.Time
	Items           []OrderItem
}

type FulfillmentOrder struct {
	ID              int
	CustomerUserID  int
	CustomerName    string
	Status          string
	PartnerStatus   string
	DeliveryStatus  string
	DeliveryNotice  string
	DeliveryAddress string
	TotalAmount     float64
	CreatedAt       time.Time
	Items           []OrderItem
}

type PartnerOrderSummary struct {
	NewCount        int
	InProgressCount int
	DispatchedCount int
	CompletedCount  int
	OverdueCount    int
}

type AuditLog struct {
	ID          int
	ActorUserID sql.NullInt64
	Action      string
	TargetType  string
	TargetID    sql.NullInt64
	Details     string
	CreatedAt   time.Time
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

func (s *Store) GetUserByEmail(email string) (*User, error) {
	row := s.DB.QueryRow(
		"SELECT id, username, email, password_hash, salt, role, slug, created_at, updated_at, deleted_at FROM users WHERE email = ? AND deleted_at IS NULL", email)

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
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (s *Store) CountUsersByRole(role string) (int, error) {
	row := s.DB.QueryRow("SELECT COUNT(*) FROM users WHERE role = ? AND deleted_at IS NULL", role)
	var count int
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) HasAdminUser() (bool, error) {
	count, err := s.CountUsersByRole("admin")
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *Store) UpdateUserPassword(userID int, passwordHash, salt string) error {
	stmt, err := s.DB.Prepare("UPDATE users SET password_hash = ?, salt = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND deleted_at IS NULL")
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(passwordHash, salt, userID)
	return err
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

	_, err = stmt.Exec(hashSessionID(sess.ID), sess.UserID, sess.ExpiresAt)
	return err
}

func (s *Store) GetSession(id string) (*Session, error) {
	row := s.DB.QueryRow("SELECT id, user_id, expires_at, created_at FROM sessions WHERE id = ?", hashSessionID(id))
	sess := &Session{}
	err := row.Scan(&sess.ID, &sess.UserID, &sess.ExpiresAt, &sess.CreatedAt)
	if err == sql.ErrNoRows {
		// Backward compatibility for legacy sessions saved as plain token IDs.
		row = s.DB.QueryRow("SELECT id, user_id, expires_at, created_at FROM sessions WHERE id = ?", id)
		err = row.Scan(&sess.ID, &sess.UserID, &sess.ExpiresAt, &sess.CreatedAt)
		if err == sql.ErrNoRows {
			return nil, nil
		}
	}
	if err != nil {
		return nil, err
	}
	return sess, nil
}

func (s *Store) DeleteSession(id string) error {
	_, err := s.DB.Exec("DELETE FROM sessions WHERE id = ? OR id = ?", hashSessionID(id), id)
	return err
}

func (s *Store) CleanupSessions() error {
	_, err := s.DB.Exec("DELETE FROM sessions WHERE expires_at < CURRENT_TIMESTAMP")
	return err
}

func (s *Store) CleanupPasswordResetTokens() error {
	_, err := s.DB.Exec(
		"DELETE FROM password_reset_tokens WHERE used_at IS NOT NULL OR expires_at < CURRENT_TIMESTAMP",
	)
	return err
}

func (s *Store) CreatePasswordResetToken(userID int, token string, expiresAt time.Time) error {
	stmt, err := s.DB.Prepare("INSERT INTO password_reset_tokens (token_hash, user_id, expires_at) VALUES (?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(hashOpaqueToken(token), userID, expiresAt)
	return err
}

func (s *Store) GetValidPasswordResetToken(token string) (*PasswordResetToken, error) {
	row := s.DB.QueryRow(
		`SELECT id, token_hash, user_id, expires_at, used_at, created_at
		 FROM password_reset_tokens
		 WHERE token_hash = ? AND used_at IS NULL AND expires_at > CURRENT_TIMESTAMP`,
		hashOpaqueToken(token),
	)

	resetToken := &PasswordResetToken{}
	err := row.Scan(
		&resetToken.ID,
		&resetToken.TokenHash,
		&resetToken.UserID,
		&resetToken.ExpiresAt,
		&resetToken.UsedAt,
		&resetToken.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return resetToken, nil
}

func (s *Store) UsePasswordResetToken(token, passwordHash, salt string) (bool, error) {
	tokenHash := hashOpaqueToken(token)
	tx, err := s.DB.Begin()
	if err != nil {
		return false, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	var userID int
	var expiresAt time.Time
	var usedAt sql.NullTime
	err = tx.QueryRow(
		"SELECT user_id, expires_at, used_at FROM password_reset_tokens WHERE token_hash = ?",
		tokenHash,
	).Scan(&userID, &expiresAt, &usedAt)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	if usedAt.Valid || expiresAt.Before(time.Now()) {
		return false, nil
	}

	result, err := tx.Exec(
		"UPDATE users SET password_hash = ?, salt = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND deleted_at IS NULL",
		passwordHash, salt, userID,
	)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected == 0 {
		return false, nil
	}

	_, err = tx.Exec(
		"UPDATE password_reset_tokens SET used_at = CURRENT_TIMESTAMP WHERE token_hash = ? AND used_at IS NULL",
		tokenHash,
	)
	if err != nil {
		return false, err
	}

	if err = tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) GetStoreSettings() (*StoreSettings, error) {
	row := s.DB.QueryRow(
		`SELECT id, store_name, store_slug, description, contact_email, initialized_at, updated_at
		 FROM store_settings
		 WHERE id = 1`,
	)
	settings := &StoreSettings{}
	err := row.Scan(
		&settings.ID,
		&settings.StoreName,
		&settings.StoreSlug,
		&settings.Description,
		&settings.ContactEmail,
		&settings.InitializedAt,
		&settings.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return settings, nil
}

func (s *Store) UpsertStoreSettings(settings StoreSettings) error {
	stmt, err := s.DB.Prepare(
		`INSERT INTO store_settings (id, store_name, store_slug, description, contact_email)
		 VALUES (1, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   store_name = excluded.store_name,
		   store_slug = excluded.store_slug,
		   description = excluded.description,
		   contact_email = excluded.contact_email,
		   updated_at = CURRENT_TIMESTAMP`,
	)
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(settings.StoreName, settings.StoreSlug, settings.Description, settings.ContactEmail)
	return err
}

func (s *Store) CountProducts() (int, error) {
	row := s.DB.QueryRow("SELECT COUNT(*) FROM products WHERE deleted_at IS NULL")
	var count int
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) CountActiveSessions() (int, error) {
	row := s.DB.QueryRow("SELECT COUNT(*) FROM sessions WHERE expires_at > CURRENT_TIMESTAMP")
	var count int
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) ListUsersByRoles(roles []string) ([]User, error) {
	if len(roles) == 0 {
		return []User{}, nil
	}

	placeholders := make([]string, len(roles))
	args := make([]interface{}, 0, len(roles))
	for i, role := range roles {
		placeholders[i] = "?"
		args = append(args, role)
	}

	query := fmt.Sprintf(
		`SELECT id, username, email, password_hash, salt, role, slug, created_at, updated_at, deleted_at
		 FROM users
		 WHERE role IN (%s) AND deleted_at IS NULL
		 ORDER BY role, username`,
		strings.Join(placeholders, ","),
	)

	rows, err := s.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := []User{}
	for rows.Next() {
		user := User{}
		err := rows.Scan(
			&user.ID,
			&user.Username,
			&user.Email,
			&user.PasswordHash,
			&user.Salt,
			&user.Role,
			&user.Slug,
			&user.CreatedAt,
			&user.UpdatedAt,
			&user.DeletedAt,
		)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, nil
}

func (s *Store) ensureCart(userID int) (int64, error) {
	var cartID int64
	err := s.DB.QueryRow("SELECT id FROM carts WHERE user_id = ?", userID).Scan(&cartID)
	if err == nil {
		return cartID, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}

	res, err := s.DB.Exec("INSERT INTO carts (user_id) VALUES (?)", userID)
	if err != nil {
		// Handle races where cart was created concurrently.
		if queryErr := s.DB.QueryRow("SELECT id FROM carts WHERE user_id = ?", userID).Scan(&cartID); queryErr == nil {
			return cartID, nil
		}
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) AddToCart(userID, productID, quantity int) error {
	if quantity <= 0 {
		return fmt.Errorf("quantity must be greater than zero")
	}
	cartID, err := s.ensureCart(userID)
	if err != nil {
		return err
	}

	_, err = s.DB.Exec(
		`INSERT INTO cart_items (cart_id, product_id, quantity)
		 VALUES (?, ?, ?)
		 ON CONFLICT(cart_id, product_id) DO UPDATE SET
		   quantity = quantity + excluded.quantity,
		   updated_at = CURRENT_TIMESTAMP`,
		cartID, productID, quantity,
	)
	return err
}

func (s *Store) UpdateCartQuantity(userID, productID, quantity int) error {
	cartID, err := s.ensureCart(userID)
	if err != nil {
		return err
	}

	if quantity <= 0 {
		_, err = s.DB.Exec("DELETE FROM cart_items WHERE cart_id = ? AND product_id = ?", cartID, productID)
		return err
	}

	_, err = s.DB.Exec(
		"UPDATE cart_items SET quantity = ?, updated_at = CURRENT_TIMESTAMP WHERE cart_id = ? AND product_id = ?",
		quantity, cartID, productID,
	)
	return err
}

func (s *Store) RemoveFromCart(userID, productID int) error {
	cartID, err := s.ensureCart(userID)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec("DELETE FROM cart_items WHERE cart_id = ? AND product_id = ?", cartID, productID)
	return err
}

func (s *Store) ClearCart(userID int) error {
	cartID, err := s.ensureCart(userID)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec("DELETE FROM cart_items WHERE cart_id = ?", cartID)
	return err
}

func (s *Store) GetCartItems(userID int) ([]CartItem, float64, error) {
	cartID, err := s.ensureCart(userID)
	if err != nil {
		return nil, 0, err
	}

	rows, err := s.DB.Query(
		`SELECT p.id, p.name, p.slug, p.price, p.stock_quantity, ci.quantity
		 FROM cart_items ci
		 JOIN products p ON p.id = ci.product_id
		 WHERE ci.cart_id = ? AND p.deleted_at IS NULL
		 ORDER BY p.name`,
		cartID,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := []CartItem{}
	subtotal := 0.0
	for rows.Next() {
		item := CartItem{}
		if err := rows.Scan(
			&item.ProductID,
			&item.ProductName,
			&item.ProductSlug,
			&item.UnitPrice,
			&item.StockQuantity,
			&item.Quantity,
		); err != nil {
			return nil, 0, err
		}
		item.ProductImageURL = "/static/images/placeholder.png"
		if imgs, imgErr := s.GetProductImages(item.ProductID); imgErr == nil && len(imgs) > 0 {
			item.ProductImageURL = imgs[0].URL
		}
		item.LineTotal = item.UnitPrice * float64(item.Quantity)
		subtotal += item.LineTotal
		items = append(items, item)
	}
	return items, subtotal, nil
}

func (s *Store) PlaceOrderFromCart(userID int, deliveryAddress string) (int64, error) {
	address := strings.TrimSpace(deliveryAddress)
	if address == "" {
		return 0, fmt.Errorf("delivery address is required")
	}

	tx, err := s.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	var cartID int64
	err = tx.QueryRow("SELECT id FROM carts WHERE user_id = ?", userID).Scan(&cartID)
	if err == sql.ErrNoRows {
		return 0, fmt.Errorf("cart is empty")
	}
	if err != nil {
		return 0, err
	}

	type cartLine struct {
		ProductID     int
		ProductName   string
		Quantity      int
		UnitPrice     float64
		StockQuantity int
	}
	lines := []cartLine{}
	rows, err := tx.Query(
		`SELECT p.id, p.name, ci.quantity, p.price, p.stock_quantity
		 FROM cart_items ci
		 JOIN products p ON p.id = ci.product_id
		 WHERE ci.cart_id = ? AND p.deleted_at IS NULL
		 ORDER BY p.name`,
		cartID,
	)
	if err != nil {
		return 0, err
	}

	total := 0.0
	for rows.Next() {
		line := cartLine{}
		if scanErr := rows.Scan(&line.ProductID, &line.ProductName, &line.Quantity, &line.UnitPrice, &line.StockQuantity); scanErr != nil {
			return 0, scanErr
		}
		if line.Quantity > line.StockQuantity {
			return 0, fmt.Errorf("insufficient stock for %s", line.ProductName)
		}
		lines = append(lines, line)
		total += line.UnitPrice * float64(line.Quantity)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	if len(lines) == 0 {
		return 0, fmt.Errorf("cart is empty")
	}

	res, err := tx.Exec(
		`INSERT INTO orders (user_id, status, partner_status, delivery_status, total_amount, delivery_address, delivery_notice)
		 VALUES (?, 'pending', 'new', 'queued', ?, ?, ?)`,
		userID, total, address, "Order received. Awaiting partner acceptance.",
	)
	if err != nil {
		return 0, err
	}
	orderID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	for _, line := range lines {
		if _, err = tx.Exec(
			`INSERT INTO order_items (order_id, product_id, product_name, quantity, unit_price)
			 VALUES (?, ?, ?, ?, ?)`,
			orderID, line.ProductID, line.ProductName, line.Quantity, line.UnitPrice,
		); err != nil {
			return 0, err
		}

		if _, err = tx.Exec(
			`UPDATE products
			 SET stock_quantity = stock_quantity - ?, updated_at = CURRENT_TIMESTAMP
			 WHERE id = ? AND stock_quantity >= ?`,
			line.Quantity, line.ProductID, line.Quantity,
		); err != nil {
			return 0, err
		}
	}

	if _, err = tx.Exec("DELETE FROM cart_items WHERE cart_id = ?", cartID); err != nil {
		return 0, err
	}

	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return orderID, nil
}

func (s *Store) listOrderItems(orderID int) ([]OrderItem, error) {
	rows, err := s.DB.Query(
		`SELECT product_id, product_name, quantity, unit_price
		 FROM order_items
		 WHERE order_id = ?
		 ORDER BY id`,
		orderID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []OrderItem{}
	for rows.Next() {
		item := OrderItem{}
		if err := rows.Scan(&item.ProductID, &item.ProductName, &item.Quantity, &item.UnitPrice); err != nil {
			return nil, err
		}
		item.LineTotal = item.UnitPrice * float64(item.Quantity)
		items = append(items, item)
	}
	return items, nil
}

func (s *Store) ListOrdersByUser(userID int) ([]CustomerOrder, error) {
	rows, err := s.DB.Query(
		`SELECT id, status, partner_status, delivery_status, delivery_address, COALESCE(delivery_notice, ''), total_amount, created_at
		 FROM orders
		 WHERE user_id = ?
		 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	orders := []CustomerOrder{}
	for rows.Next() {
		order := CustomerOrder{}
		if err := rows.Scan(
			&order.ID,
			&order.Status,
			&order.PartnerStatus,
			&order.DeliveryStatus,
			&order.DeliveryAddress,
			&order.DeliveryNotice,
			&order.TotalAmount,
			&order.CreatedAt,
		); err != nil {
			return nil, err
		}
		items, err := s.listOrderItems(order.ID)
		if err != nil {
			return nil, err
		}
		order.Items = items
		orders = append(orders, order)
	}
	return orders, nil
}

func (s *Store) ListOrdersForFulfillment() ([]FulfillmentOrder, error) {
	rows, err := s.DB.Query(
		`SELECT o.id, o.user_id, u.username, o.status, o.partner_status, o.delivery_status, COALESCE(o.delivery_notice, ''), o.delivery_address, o.total_amount, o.created_at
		 FROM orders o
		 JOIN users u ON u.id = o.user_id
		 WHERE o.status != 'cancelled'
		 ORDER BY o.created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	orders := []FulfillmentOrder{}
	for rows.Next() {
		order := FulfillmentOrder{}
		if err := rows.Scan(
			&order.ID,
			&order.CustomerUserID,
			&order.CustomerName,
			&order.Status,
			&order.PartnerStatus,
			&order.DeliveryStatus,
			&order.DeliveryNotice,
			&order.DeliveryAddress,
			&order.TotalAmount,
			&order.CreatedAt,
		); err != nil {
			return nil, err
		}
		items, err := s.listOrderItems(order.ID)
		if err != nil {
			return nil, err
		}
		order.Items = items
		orders = append(orders, order)
	}
	return orders, nil
}

func (s *Store) UpdateOrderFulfillment(orderID int, partnerStatus, deliveryStatus, deliveryNotice string) error {
	var currentPartnerStatus string
	var currentDeliveryStatus string
	err := s.DB.QueryRow(
		"SELECT partner_status, delivery_status FROM orders WHERE id = ?",
		orderID,
	).Scan(&currentPartnerStatus, &currentDeliveryStatus)
	if err == sql.ErrNoRows {
		return fmt.Errorf("order not found")
	}
	if err != nil {
		return err
	}

	if !isAllowedPartnerTransition(currentPartnerStatus, partnerStatus) {
		return fmt.Errorf("invalid partner status transition")
	}
	if !isAllowedDeliveryTransition(currentDeliveryStatus, deliveryStatus) {
		return fmt.Errorf("invalid delivery status transition")
	}

	statusMap := map[string]string{
		"new":        "pending",
		"accepted":   "processing",
		"packing":    "processing",
		"dispatched": "shipped",
		"completed":  "delivered",
		"cancelled":  "cancelled",
	}
	nextStatus, ok := statusMap[partnerStatus]
	if !ok {
		return fmt.Errorf("invalid partner status")
	}

	validDelivery := map[string]bool{
		"queued":     true,
		"processing": true,
		"shipped":    true,
		"delivered":  true,
	}
	if !validDelivery[deliveryStatus] {
		return fmt.Errorf("invalid delivery status")
	}

	_, err = s.DB.Exec(
		`UPDATE orders
		 SET status = ?, partner_status = ?, delivery_status = ?, delivery_notice = ?, updated_at = CURRENT_TIMESTAMP
		 WHERE id = ?`,
		nextStatus, partnerStatus, deliveryStatus, strings.TrimSpace(deliveryNotice), orderID,
	)
	return err
}

func isAllowedPartnerTransition(from, to string) bool {
	allowed := map[string]map[string]bool{
		"new": {
			"new":       true,
			"accepted":  true,
			"cancelled": true,
		},
		"accepted": {
			"accepted":  true,
			"packing":   true,
			"cancelled": true,
		},
		"packing": {
			"packing":    true,
			"dispatched": true,
			"cancelled":  true,
		},
		"dispatched": {
			"dispatched": true,
			"completed":  true,
		},
		"completed": {
			"completed": true,
		},
		"cancelled": {
			"cancelled": true,
		},
	}
	return allowed[from][to]
}

func isAllowedDeliveryTransition(from, to string) bool {
	allowed := map[string]map[string]bool{
		"queued": {
			"queued":     true,
			"processing": true,
			"shipped":    true,
			"delivered":  true,
		},
		"processing": {
			"processing": true,
			"shipped":    true,
			"delivered":  true,
		},
		"shipped": {
			"shipped":   true,
			"delivered": true,
		},
		"delivered": {
			"delivered": true,
		},
	}
	return allowed[from][to]
}

func (s *Store) GetPartnerOrderSummary() (PartnerOrderSummary, error) {
	summary := PartnerOrderSummary{}

	row := s.DB.QueryRow(
		`SELECT
		    SUM(CASE WHEN partner_status = 'new' THEN 1 ELSE 0 END),
		    SUM(CASE WHEN partner_status IN ('accepted', 'packing') THEN 1 ELSE 0 END),
		    SUM(CASE WHEN partner_status = 'dispatched' THEN 1 ELSE 0 END),
		    SUM(CASE WHEN partner_status = 'completed' THEN 1 ELSE 0 END)
		 FROM orders
		 WHERE status != 'cancelled'`,
	)
	var newCount, inProgress, dispatched, completed sql.NullInt64
	if err := row.Scan(&newCount, &inProgress, &dispatched, &completed); err != nil {
		return summary, err
	}

	if newCount.Valid {
		summary.NewCount = int(newCount.Int64)
	}
	if inProgress.Valid {
		summary.InProgressCount = int(inProgress.Int64)
	}
	if dispatched.Valid {
		summary.DispatchedCount = int(dispatched.Int64)
	}
	if completed.Valid {
		summary.CompletedCount = int(completed.Int64)
	}

	overdueRow := s.DB.QueryRow(
		`SELECT COUNT(*)
		 FROM orders
		 WHERE partner_status IN ('new', 'accepted')
		   AND status != 'cancelled'
		   AND created_at <= datetime('now', '-2 hours')`,
	)
	if err := overdueRow.Scan(&summary.OverdueCount); err != nil {
		return summary, err
	}

	return summary, nil
}

func (s *Store) ListAllOrders() ([]FulfillmentOrder, error) {
	rows, err := s.DB.Query(
		`SELECT o.id, o.user_id, u.username, o.status, o.partner_status, o.delivery_status, COALESCE(o.delivery_notice, ''), o.delivery_address, o.total_amount, o.created_at
		 FROM orders o
		 JOIN users u ON u.id = o.user_id
		 ORDER BY o.created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	orders := []FulfillmentOrder{}
	for rows.Next() {
		order := FulfillmentOrder{}
		if err := rows.Scan(
			&order.ID,
			&order.CustomerUserID,
			&order.CustomerName,
			&order.Status,
			&order.PartnerStatus,
			&order.DeliveryStatus,
			&order.DeliveryNotice,
			&order.DeliveryAddress,
			&order.TotalAmount,
			&order.CreatedAt,
		); err != nil {
			return nil, err
		}
		items, err := s.listOrderItems(order.ID)
		if err != nil {
			return nil, err
		}
		order.Items = items
		orders = append(orders, order)
	}
	return orders, nil
}

func (s *Store) UpdateOrderByAdmin(orderID int, status, partnerStatus, deliveryStatus, deliveryNotice string) error {
	validStatus := map[string]bool{
		"pending":    true,
		"processing": true,
		"shipped":    true,
		"delivered":  true,
		"cancelled":  true,
	}
	validPartner := map[string]bool{
		"new":        true,
		"accepted":   true,
		"packing":    true,
		"dispatched": true,
		"completed":  true,
		"cancelled":  true,
	}
	validDelivery := map[string]bool{
		"queued":     true,
		"processing": true,
		"shipped":    true,
		"delivered":  true,
	}
	if !validStatus[status] || !validPartner[partnerStatus] || !validDelivery[deliveryStatus] {
		return fmt.Errorf("invalid order state")
	}

	_, err := s.DB.Exec(
		`UPDATE orders
		 SET status = ?, partner_status = ?, delivery_status = ?, delivery_notice = ?, updated_at = CURRENT_TIMESTAMP
		 WHERE id = ?`,
		status, partnerStatus, deliveryStatus, strings.TrimSpace(deliveryNotice), orderID,
	)
	return err
}

func (s *Store) GetOrderStatusCounts() (map[string]int, error) {
	rows, err := s.DB.Query("SELECT status, COUNT(*) FROM orders GROUP BY status")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := map[string]int{
		"pending":    0,
		"processing": 0,
		"shipped":    0,
		"delivered":  0,
		"cancelled":  0,
	}
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		counts[status] = count
	}
	return counts, nil
}

func (s *Store) SumOrderRevenue() (float64, error) {
	row := s.DB.QueryRow("SELECT COALESCE(SUM(total_amount), 0) FROM orders WHERE status != 'cancelled'")
	var total float64
	if err := row.Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

func (s *Store) CreateAuditLog(actorUserID int, action, targetType string, targetID int, details string) error {
	if strings.TrimSpace(action) == "" || strings.TrimSpace(targetType) == "" {
		return fmt.Errorf("action and target_type are required")
	}

	var actor sql.NullInt64
	if actorUserID > 0 {
		actor = sql.NullInt64{Int64: int64(actorUserID), Valid: true}
	}
	var target sql.NullInt64
	if targetID > 0 {
		target = sql.NullInt64{Int64: int64(targetID), Valid: true}
	}

	_, err := s.DB.Exec(
		"INSERT INTO audit_logs (actor_user_id, action, target_type, target_id, details) VALUES (?, ?, ?, ?, ?)",
		actor, strings.TrimSpace(action), strings.TrimSpace(targetType), target, strings.TrimSpace(details),
	)
	return err
}

func (s *Store) ListAuditLogs(limit int) ([]AuditLog, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := s.DB.Query(
		"SELECT id, actor_user_id, action, target_type, target_id, COALESCE(details, ''), created_at FROM audit_logs ORDER BY created_at DESC, id DESC LIMIT ?",
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	logs := []AuditLog{}
	for rows.Next() {
		entry := AuditLog{}
		if err := rows.Scan(
			&entry.ID,
			&entry.ActorUserID,
			&entry.Action,
			&entry.TargetType,
			&entry.TargetID,
			&entry.Details,
			&entry.CreatedAt,
		); err != nil {
			return nil, err
		}
		logs = append(logs, entry)
	}
	return logs, nil
}

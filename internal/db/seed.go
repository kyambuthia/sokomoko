package db

import (
	"database/sql"
	"log"
	"os"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

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

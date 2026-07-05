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

func (s *Store) SeedPartners() {
	partners := []struct {
		Username string
		Email    string
		Password string
	}{
		{Username: "urban_goods", Email: "urban@sokomoko.com", Password: "partnerpass123"},
		{Username: "nature_supply", Email: "nature@sokomoko.com", Password: "partnerpass123"},
		{Username: "desk_essentials", Email: "desk@sokomoko.com", Password: "partnerpass123"},
		{Username: "home_craft", Email: "craft@sokomoko.com", Password: "partnerpass123"},
	}

	for _, p := range partners {
		var count int
		err := s.DB.QueryRow("SELECT COUNT(*) FROM users WHERE username = ?", p.Username).Scan(&count)
		if err != nil {
			log.Printf("Error checking for partner %s: %v", p.Username, err)
			continue
		}

		if count > 0 {
			continue
		}

		salt, err := generateSalt()
		if err != nil {
			log.Printf("Error generating salt for %s: %v", p.Username, err)
			continue
		}

		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(p.Password+salt), bcrypt.DefaultCost)
		if err != nil {
			log.Printf("Error hashing password for %s: %v", p.Username, err)
			continue
		}

		user := User{
			Username:     p.Username,
			Email:        p.Email,
			PasswordHash: string(hashedPassword),
			Salt:         salt,
			Role:         "partner",
			Slug:         p.Username,
		}

		_, err = s.CreateUser(user)
		if err != nil {
			log.Printf("Error seeding partner %s: %v", p.Username, err)
		} else {
			log.Printf("Partner created: %s", p.Username)
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
		Partner      string
	}{
		// Electronics demo set
		{
			Name:         "Flagship Smartphone",
			Slug:         "flagship-smartphone",
			Description:  "Premium studio-ready smartphone with a sleek black finish and a clean front profile.",
			Price:        999.00,
			Stock:        18,
			Category:     "Electronics",
			CategorySlug: "electronics",
			ImageURL:     "/static/images/products/electronics/flagship-smartphone/front.png",
			Partner:      "urban_goods",
		},
		{
			Name:         "Android Smartphone",
			Slug:         "android-smartphone",
			Description:  "Sleek silver smartphone with a bold camera array and a premium rear finish.",
			Price:        899.00,
			Stock:        22,
			Category:     "Electronics",
			CategorySlug: "electronics",
			ImageURL:     "/static/images/products/electronics/android-smartphone/rear.png",
			Partner:      "urban_goods",
		},
		{
			Name:         "Next-Gen Game Console",
			Slug:         "next-gen-game-console",
			Description:  "Modern white game console bundle with matching controller and a clean studio presentation.",
			Price:        599.00,
			Stock:        12,
			Category:     "Electronics",
			CategorySlug: "electronics",
			ImageURL:     "/static/images/products/electronics/next-gen-game-console/bundle.png",
			Partner:      "desk_essentials",
		},
	}

	for _, item := range catalog {
		categoryID, err := s.ensureCategory(item.Category, item.CategorySlug)
		if err != nil {
			log.Printf("Skipping seed product %s: failed to ensure category: %v", item.Slug, err)
			continue
		}

		partnerID, err := s.ensurePartner(item.Partner)
		if err != nil {
			log.Printf("Skipping seed product %s: failed to ensure partner: %v", item.Slug, err)
			continue
		}

		productID, existed, err := s.ensureProduct(item, categoryID, partnerID)
		if err != nil {
			log.Printf("Skipping seed product %s: %v", item.Slug, err)
			continue
		}

		if err := s.ensurePrimaryImage(productID, item.ImageURL); err != nil {
			log.Printf("Seed product image warning for %s: %v", item.Slug, err)
			continue
		}

		if !existed {
			log.Printf("Seeded product: %s (by %s)", item.Name, item.Partner)
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

func (s *Store) ensurePartner(username string) (int64, error) {
	var id int64
	err := s.DB.QueryRow("SELECT id FROM users WHERE username = ? AND role = 'partner'", username).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}
	return 0, nil
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
	Partner      string
}, categoryID int64, partnerID int64) (int, bool, error) {
	var id int
	err := s.DB.QueryRow("SELECT id FROM products WHERE slug = ? AND deleted_at IS NULL", item.Slug).Scan(&id)
	if err == nil {
		return id, true, nil
	}
	if err != sql.ErrNoRows {
		return 0, false, err
	}

	var catID, pID sql.NullInt64
	if categoryID > 0 {
		catID = sql.NullInt64{Int64: categoryID, Valid: true}
	}
	if partnerID > 0 {
		pID = sql.NullInt64{Int64: partnerID, Valid: true}
	}

	newID, err := s.CreateProduct(Product{
		Name:          item.Name,
		Slug:          item.Slug,
		Description:   item.Description,
		Price:         item.Price,
		StockQuantity: item.Stock,
		CategoryID:    catID,
		PartnerID:     pID,
	})
	if err != nil {
		return 0, false, err
	}
	return int(newID), false, nil
}

func (s *Store) ensurePrimaryImage(productID int, imageURL string) error {
	var existingCount int
	err := s.DB.QueryRow(
		"SELECT COUNT(*) FROM product_images WHERE product_id = ?",
		productID,
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

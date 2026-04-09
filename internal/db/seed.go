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
		// Electronics - Urban Goods
		{
			Name:         "Wireless Earbuds Pro",
			Slug:         "wireless-earbuds-pro",
			Description:  "Premium wireless earbuds with active noise cancellation and 24-hour battery life.",
			Price:        149.99,
			Stock:        35,
			Category:     "Electronics",
			CategorySlug: "electronics",
			ImageURL:     "https://picsum.photos/seed/earbuds/400/400",
			Partner:      "urban_goods",
		},
		{
			Name:         "Mechanical Keyboard",
			Slug:         "mechanical-keyboard",
			Description:  "Compact 75% mechanical keyboard with hot-swappable switches and RGB backlighting.",
			Price:        89.00,
			Stock:        22,
			Category:     "Electronics",
			CategorySlug: "electronics",
			ImageURL:     "https://picsum.photos/seed/keyboard/400/400",
			Partner:      "urban_goods",
		},
		{
			Name:         "USB-C Hub 7-in-1",
			Slug:         "usb-c-hub-7in1",
			Description:  "Multi-port adapter with HDMI, USB-A, SD card reader, and ethernet.",
			Price:        45.50,
			Stock:        50,
			Category:     "Electronics",
			CategorySlug: "electronics",
			ImageURL:     "https://picsum.photos/seed/usbhub/400/400",
			Partner:      "desk_essentials",
		},

		// Home & Kitchen - Nature Supply
		{
			Name:         "Stainless Steel Kettle",
			Slug:         "stainless-steel-kettle",
			Description:  "1.7L electric kettle with auto-shutoff and boil-dry protection.",
			Price:        49.99,
			Stock:        40,
			Category:     "Home & Kitchen",
			CategorySlug: "home-kitchen",
			ImageURL:     "https://picsum.photos/seed/kettle/400/400",
			Partner:      "nature_supply",
		},
		{
			Name:         "Bamboo Cutting Board Set",
			Slug:         "bamboo-cutting-board-set",
			Description:  "Set of 3 organic bamboo boards with juice grooves and hanging holes.",
			Price:        32.00,
			Stock:        28,
			Category:     "Home & Kitchen",
			CategorySlug: "home-kitchen",
			ImageURL:     "https://picsum.photos/seed/bamboo/400/400",
			Partner:      "nature_supply",
		},
		{
			Name:         "Ceramic Coffee Mug Set",
			Slug:         "ceramic-mug-set",
			Description:  "Handcrafted ceramic mugs in matte finish, set of 4.",
			Price:        38.00,
			Stock:        45,
			Category:     "Home & Kitchen",
			CategorySlug: "home-kitchen",
			ImageURL:     "https://picsum.photos/seed/mugs/400/400",
			Partner:      "home_craft",
		},

		// Desk & Office - Desk Essentials
		{
			Name:         "Adjustable Monitor Stand",
			Slug:         "adjustable-monitor-stand",
			Description:  "Bamboo monitor riser with drawer storage and adjustable height.",
			Price:        65.00,
			Stock:        30,
			Category:     "Desk & Office",
			CategorySlug: "desk-office",
			ImageURL:     "https://picsum.photos/seed/monitorstand/400/400",
			Partner:      "desk_essentials",
		},
		{
			Name:         "LED Desk Lamp",
			Slug:         "led-desk-lamp",
			Description:  "Minimalist LED lamp with touch dimmer and USB charging port.",
			Price:        55.00,
			Stock:        38,
			Category:     "Desk & Office",
			CategorySlug: "desk-office",
			ImageURL:     "https://picsum.photos/seed/lamp/400/400",
			Partner:      "desk_essentials",
		},
		{
			Name:         "Desk Organizer Set",
			Slug:         "desk-organizer-set",
			Description:  "Acrylic desktop organizer with pen holder, phone stand, and tray.",
			Price:        28.50,
			Stock:        55,
			Category:     "Desk & Office",
			CategorySlug: "desk-office",
			ImageURL:     "https://picsum.photos/seed/organizer/400/400",
			Partner:      "home_craft",
		},

		// Bags & Accessories - Urban Goods
		{
			Name:         "Canvas Laptop Backpack",
			Slug:         "canvas-laptop-backpack",
			Description:  "Water-resistant canvas backpack with padded laptop compartment.",
			Price:        75.00,
			Stock:        25,
			Category:     "Bags & Accessories",
			CategorySlug: "bags-accessories",
			ImageURL:     "https://picsum.photos/seed/backpack/400/400",
			Partner:      "urban_goods",
		},
		{
			Name:         "Leather Messenger Bag",
			Slug:         "leather-messenger-bag",
			Description:  "Full-grain leather crossbody bag with antique brass hardware.",
			Price:        125.00,
			Stock:        15,
			Category:     "Bags & Accessories",
			CategorySlug: "bags-accessories",
			ImageURL:     "https://picsum.photos/seed/messenger/400/400",
			Partner:      "home_craft",
		},
		{
			Name:         "Minimalist Wallet",
			Slug:         "minimalist-wallet",
			Description:  "Slim cardholder in vegetable-tanned leather, holds 8 cards.",
			Price:        42.00,
			Stock:        60,
			Category:     "Bags & Accessories",
			CategorySlug: "bags-accessories",
			ImageURL:     "https://picsum.photos/seed/wallet/400/400",
			Partner:      "urban_goods",
		},

		// Personal Care - Nature Supply
		{
			Name:         "Bamboo Toothbrush Set",
			Slug:         "bamboo-toothbrush-set",
			Description:  "Pack of 4 biodegradable bamboo toothbrushes with soft bristles.",
			Price:        12.99,
			Stock:        100,
			Category:     "Personal Care",
			CategorySlug: "personal-care",
			ImageURL:     "https://picsum.photos/seed/toothbrush/400/400",
			Partner:      "nature_supply",
		},
		{
			Name:         "Natural Lip Balm Trio",
			Slug:         "natural-lip-balm-trio",
			Description:  "Organic beeswax lip balms in mint, lavender, and unscented.",
			Price:        15.00,
			Stock:        80,
			Category:     "Personal Care",
			CategorySlug: "personal-care",
			ImageURL:     "https://picsum.photos/seed/lipbalm/400/400",
			Partner:      "nature_supply",
		},

		// Stationery - Home Craft
		{
			Name:         "Leather Journal",
			Slug:         "leather-journal",
			Description:  "Handbound dotted journal with refillable pages and leather cover.",
			Price:        35.00,
			Stock:        40,
			Category:     "Stationery",
			CategorySlug: "stationery",
			ImageURL:     "https://picsum.photos/seed/journal/400/400",
			Partner:      "home_craft",
		},
		{
			Name:         "Brass Pen Set",
			Slug:         "brass-pen-set",
			Description:  "Tactical-style ballpoint pens in solid brass, set of 2.",
			Price:        48.00,
			Stock:        30,
			Category:     "Stationery",
			CategorySlug: "stationery",
			ImageURL:     "https://picsum.photos/seed/brasspen/400/400",
			Partner:      "desk_essentials",
		},
		{
			Name:         "Washi Tape Collection",
			Slug:         "washi-tape-collection",
			Description:  "Set of 6 botanical-themed washi tapes on wooden dispensers.",
			Price:        22.00,
			Stock:        65,
			Category:     "Stationery",
			CategorySlug: "stationery",
			ImageURL:     "https://picsum.photos/seed/washitape/400/400",
			Partner:      "home_craft",
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

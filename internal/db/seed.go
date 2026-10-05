package db

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/kyambuthia/sokomoko/internal/money"
)

// SeedAdmin creates the first admin account from ADMIN_USERNAME, ADMIN_EMAIL and
// ADMIN_PASSWORD when no admin exists. Without ADMIN_PASSWORD it does nothing;
// the admin host's /setup flow is then used instead.
func (s *Store) SeedAdmin() {
	hasAdmin, err := s.HasAdminUser()
	if err != nil {
		log.Printf("seed: check for admin user: %v", err)
		return
	}
	if hasAdmin {
		return
	}

	password := os.Getenv("ADMIN_PASSWORD")
	if strings.TrimSpace(password) == "" {
		log.Printf("seed: ADMIN_PASSWORD not set; skipping admin bootstrap user")
		return
	}
	if len(password) < 10 || len(password) > 72 {
		log.Printf("seed: ADMIN_PASSWORD must be 10-72 bytes; skipping admin bootstrap user")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("seed: hash admin password: %v", err)
		return
	}

	username := strings.TrimSpace(os.Getenv("ADMIN_USERNAME"))
	if username == "" {
		username = "admin"
	}
	email := strings.TrimSpace(os.Getenv("ADMIN_EMAIL"))
	if email == "" {
		email = "admin@sokomoko.local"
	}

	if _, err := s.CreateUser(User{Username: username, Email: email, PasswordHash: string(hash), Role: RoleAdmin}); err != nil {
		log.Printf("seed: create admin user: %v", err)
		return
	}
	log.Printf("seed: admin bootstrap user created: %s", username)
}

type seedPartner struct {
	Name  string
	Slug  string
	Email string
}

var demoPartners = []seedPartner{
	{Name: "Urban Goods", Slug: "urban_goods", Email: "urban@sokomoko.local"},
	{Name: "Nature Supply", Slug: "nature_supply", Email: "nature@sokomoko.local"},
	{Name: "Desk Essentials", Slug: "desk_essentials", Email: "desk@sokomoko.local"},
	{Name: "Home Craft", Slug: "home_craft", Email: "craft@sokomoko.local"},
}

// SeedPartners creates the demo vendor records. Partners are catalog entities,
// not login accounts, so no credentials are created.
func (s *Store) SeedPartners() {
	for _, p := range demoPartners {
		if _, err := s.UpsertPartner(Partner{Name: p.Name, Slug: p.Slug, ContactEmail: p.Email}); err != nil {
			log.Printf("seed: partner %s: %v", p.Slug, err)
		}
	}
}

type seedProduct struct {
	Name         string
	Slug         string
	Description  string
	Price        money.Cents
	Stock        int
	Category     string
	CategorySlug string
	ImageURL     string
	Partner      string
}

var demoCatalog = []seedProduct{
	{
		Name:         "Flagship Smartphone",
		Slug:         "flagship-smartphone",
		Description:  "Premium studio-ready smartphone with a sleek black finish and a clean front profile.",
		Price:        99900,
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
		Price:        89900,
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
		Price:        59900,
		Stock:        12,
		Category:     "Electronics",
		CategorySlug: "electronics",
		ImageURL:     "/static/images/products/electronics/next-gen-game-console/bundle.png",
		Partner:      "desk_essentials",
	},
}

// SeedInitialCatalog creates the demo catalog. Existing products are left
// untouched so the seed can be re-run safely.
func (s *Store) SeedInitialCatalog() {
	for _, item := range demoCatalog {
		if err := s.seedProduct(item); err != nil {
			log.Printf("seed: product %s: %v", item.Slug, err)
		}
	}
}

func (s *Store) seedProduct(item seedProduct) error {
	existing, err := s.GetProductBySlug(item.Slug)
	if err != nil {
		return err
	}
	if existing != nil {
		return nil
	}

	category, err := s.GetCategoryBySlug(item.CategorySlug)
	if err != nil {
		return err
	}
	var categoryID int64
	if category != nil {
		categoryID = int64(category.ID)
	} else {
		categoryID, err = s.CreateCategory(Category{Name: item.Category, Slug: item.CategorySlug, Description: item.Category + " products"})
		if err != nil {
			return fmt.Errorf("create category: %w", err)
		}
	}

	var partnerID sql.NullInt64
	partner, err := s.GetPartnerBySlug(item.Partner)
	if err != nil {
		return err
	}
	if partner != nil {
		partnerID = sql.NullInt64{Int64: int64(partner.ID), Valid: true}
	}

	_, err = s.CreateProduct(Product{
		Name:          item.Name,
		Slug:          item.Slug,
		Description:   item.Description,
		Price:         item.Price,
		StockQuantity: item.Stock,
		CategoryID:    sql.NullInt64{Int64: categoryID, Valid: true},
		PartnerID:     partnerID,
		Images:        []ProductImage{{URL: item.ImageURL, AltText: item.Name}},
	})
	if errors.Is(err, ErrProductSlugConflict) {
		return nil
	}
	if err == nil {
		log.Printf("seed: product created: %s", item.Name)
	}
	return err
}

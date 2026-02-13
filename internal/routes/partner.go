package routes

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/kyambuthia/sokomoko/internal/app"
	"github.com/kyambuthia/sokomoko/internal/auth"
	"github.com/kyambuthia/sokomoko/internal/db"
)

type PartnerSetupData struct {
	Title    string
	Heading  string
	Message  string
	Error    string
	Settings db.StoreSettings
}

type PartnerDashboardData struct {
	Title        string
	StoreName    string
	StoreSlug    string
	Description  string
	ContactEmail string
	ProductCount int
	Role         string
	Username     string
}

type PartnerProductsData struct {
	Title      string
	StoreName  string
	Products   []db.Product
	Message    string
	Error      string
	Categories []db.Category
}

type PartnerProductNewData struct {
	Title       string
	Message     string
	Error       string
	Categories  []db.Category
	DefaultName string
}

func partnerSlugify(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	var b strings.Builder
	prevDash := false
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prevDash = false
			continue
		}
		if !prevDash {
			b.WriteByte('-')
			prevDash = true
		}
	}
	slug := strings.Trim(b.String(), "-")
	return slug
}

func PartnerRoot(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		settings, err := a.Store.GetStoreSettings()
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if settings == nil {
			http.Redirect(w, r, "/setup", http.StatusFound)
			return
		}
		http.Redirect(w, r, "/dashboard", http.StatusFound)
	}
}

func PartnerSetup(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		existing, err := a.Store.GetStoreSettings()
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		data := PartnerSetupData{
			Title:   "Store Setup",
			Heading: "Partner Store Setup",
		}
		if existing != nil {
			data.Settings = *existing
		}

		if r.Method == http.MethodGet {
			a.Render(w, a.Templates.PartnerSetup, data)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		storeName := strings.TrimSpace(r.FormValue("store_name"))
		storeSlug := strings.ToLower(strings.TrimSpace(r.FormValue("store_slug")))
		description := strings.TrimSpace(r.FormValue("description"))
		contactEmail := strings.ToLower(strings.TrimSpace(r.FormValue("contact_email")))
		if storeName == "" || storeSlug == "" || contactEmail == "" {
			data.Error = "Store name, slug, and contact email are required."
			data.Settings = db.StoreSettings{
				StoreName:    storeName,
				StoreSlug:    storeSlug,
				Description:  description,
				ContactEmail: contactEmail,
			}
			a.Render(w, a.Templates.PartnerSetup, data)
			return
		}

		err = a.Store.UpsertStoreSettings(db.StoreSettings{
			StoreName:    storeName,
			StoreSlug:    storeSlug,
			Description:  description,
			ContactEmail: contactEmail,
		})
		if err != nil {
			data.Error = "Unable to save store setup. Ensure slug is unique."
			data.Settings = db.StoreSettings{
				StoreName:    storeName,
				StoreSlug:    storeSlug,
				Description:  description,
				ContactEmail: contactEmail,
			}
			a.Render(w, a.Templates.PartnerSetup, data)
			return
		}

		data.Message = "Store setup saved successfully."
		data.Settings = db.StoreSettings{
			StoreName:    storeName,
			StoreSlug:    storeSlug,
			Description:  description,
			ContactEmail: contactEmail,
		}
		a.Render(w, a.Templates.PartnerSetup, data)
	}
}

func PartnerDashboard(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		settings, err := a.Store.GetStoreSettings()
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if settings == nil {
			http.Redirect(w, r, "/setup", http.StatusFound)
			return
		}

		products, err := a.Store.GetAllProducts()
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		role := "staff"
		username := ""
		user := auth.GetUserFromContext(r.Context())
		if user != nil {
			role = user.Role
			username = user.Username
		}

		a.Render(w, a.Templates.PartnerDashboard, PartnerDashboardData{
			Title:        "Partner Dashboard",
			StoreName:    settings.StoreName,
			StoreSlug:    settings.StoreSlug,
			Description:  settings.Description,
			ContactEmail: settings.ContactEmail,
			ProductCount: len(products),
			Role:         role,
			Username:     username,
		})
	}
}

func PartnerProducts(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		settings, err := a.Store.GetStoreSettings()
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if settings == nil {
			http.Redirect(w, r, "/setup", http.StatusFound)
			return
		}

		products, err := a.Store.GetAllProducts()
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		categories, _ := a.Store.GetAllCategories()

		data := PartnerProductsData{
			Title:      "Partner Products",
			StoreName:  settings.StoreName,
			Products:   products,
			Categories: categories,
		}
		a.Render(w, a.Templates.PartnerProducts, data)
	}
}

func PartnerProductNew(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		settings, err := a.Store.GetStoreSettings()
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if settings == nil {
			http.Redirect(w, r, "/setup", http.StatusFound)
			return
		}

		categories, _ := a.Store.GetAllCategories()
		data := PartnerProductNewData{
			Title:      "Add Product",
			Categories: categories,
		}

		if r.Method == http.MethodGet {
			a.Render(w, a.Templates.PartnerProductNew, data)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		name := strings.TrimSpace(r.FormValue("name"))
		description := strings.TrimSpace(r.FormValue("description"))
		priceRaw := strings.TrimSpace(r.FormValue("price"))
		stockRaw := strings.TrimSpace(r.FormValue("stock_quantity"))
		categoryIDRaw := strings.TrimSpace(r.FormValue("category_id"))

		data.DefaultName = name
		if name == "" || priceRaw == "" || stockRaw == "" {
			data.Error = "Name, price and stock are required"
			a.Render(w, a.Templates.PartnerProductNew, data)
			return
		}

		price, err := strconv.ParseFloat(priceRaw, 64)
		if err != nil || price < 0 {
			data.Error = "Price must be a non-negative number"
			a.Render(w, a.Templates.PartnerProductNew, data)
			return
		}
		stock, err := strconv.Atoi(stockRaw)
		if err != nil || stock < 0 {
			data.Error = "Stock must be a non-negative integer"
			a.Render(w, a.Templates.PartnerProductNew, data)
			return
		}

		slugBase := partnerSlugify(name)
		if slugBase == "" {
			slugBase = "product"
		}

		product := db.Product{
			Name:          name,
			Slug:          slugBase,
			Description:   description,
			Price:         price,
			StockQuantity: stock,
		}

		if categoryIDRaw != "" {
			categoryID, parseErr := strconv.Atoi(categoryIDRaw)
			if parseErr == nil && categoryID > 0 {
				product.CategoryID = sql.NullInt64{Int64: int64(categoryID), Valid: true}
			}
		}

		created := false
		for attempt := 0; attempt < 10; attempt++ {
			if attempt > 0 {
				product.Slug = slugBase + "-" + strconv.FormatInt(time.Now().Unix(), 10) + "-" + strconv.Itoa(attempt)
			}
			if _, err := a.Store.CreateProduct(product); err != nil {
				if strings.Contains(strings.ToLower(err.Error()), "products.slug") {
					continue
				}
				data.Error = "Unable to create product"
				a.Render(w, a.Templates.PartnerProductNew, data)
				return
			}
			created = true
			break
		}
		if !created {
			data.Error = "Unable to create product slug"
			a.Render(w, a.Templates.PartnerProductNew, data)
			return
		}

		http.Redirect(w, r, "/products", http.StatusFound)
	}
}

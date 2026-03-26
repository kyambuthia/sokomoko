package routes

import (
	"errors"
	"net/http"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/app"
	partnersvc "github.com/kyambuthia/sokomoko/internal/service/partner"
)

type PartnerSetupData struct {
	Title    string
	Heading  string
	Message  string
	Error    string
	Settings partnersvc.StoreSettings
}

type PartnerDashboardData struct {
	Title            string
	StoreName        string
	StoreSlug        string
	Description      string
	ContactEmail     string
	ProductCount     int
	NewOrders        int
	InProgressOrders int
	DispatchedOrders int
	CompletedOrders  int
	OverdueOrders    int
	Role             string
	Username         string
}

type PartnerProductsData struct {
	Title      string
	StoreName  string
	Products   []partnersvc.Product
	Message    string
	Error      string
	Categories []partnersvc.Category
}

type PartnerProductNewData struct {
	Title       string
	Message     string
	Error       string
	Categories  []partnersvc.Category
	DefaultName string
}

func PartnerRoot(a *app.App) http.HandlerFunc {
	svc := a.Partner

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		settings, err := svc.StoreSettings()
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
	svc := a.Partner

	return func(w http.ResponseWriter, r *http.Request) {
		existing, err := svc.StoreSettings()
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

		settings, err := svc.SaveStoreSettings(partnersvc.StoreSettingsInput{
			StoreName:    r.FormValue("store_name"),
			StoreSlug:    r.FormValue("store_slug"),
			Description:  r.FormValue("description"),
			ContactEmail: r.FormValue("contact_email"),
		})
		switch {
		case errors.Is(err, partnersvc.ErrMissingStoreFields):
			data.Error = "Store name, slug, and contact email are required."
			data.Settings = settings
			a.Render(w, a.Templates.PartnerSetup, data)
			return
		case err != nil:
			data.Error = "Unable to save store setup. Ensure slug is unique."
			data.Settings = settings
			a.Render(w, a.Templates.PartnerSetup, data)
			return
		}

		data.Message = "Store setup saved successfully."
		data.Settings = settings
		a.Render(w, a.Templates.PartnerSetup, data)
	}
}

func PartnerDashboard(a *app.App) http.HandlerFunc {
	svc := a.Partner

	return func(w http.ResponseWriter, r *http.Request) {
		view, err := svc.Dashboard()
		switch {
		case errors.Is(err, partnersvc.ErrStoreNotConfigured):
			http.Redirect(w, r, "/setup", http.StatusFound)
			return
		case err != nil:
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		role, username := partnerIdentity(r)
		a.Render(w, a.Templates.PartnerDashboard, partnerDashboardPage(view, role, username))
	}
}

func PartnerProducts(a *app.App) http.HandlerFunc {
	svc := a.Partner

	return func(w http.ResponseWriter, r *http.Request) {
		view, err := svc.Products()
		switch {
		case errors.Is(err, partnersvc.ErrStoreNotConfigured):
			http.Redirect(w, r, "/setup", http.StatusFound)
			return
		case err != nil:
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		a.Render(w, a.Templates.PartnerProducts, partnerProductsPage(view))
	}
}

func PartnerProductNew(a *app.App) http.HandlerFunc {
	svc := a.Partner

	return func(w http.ResponseWriter, r *http.Request) {
		view, err := svc.Products()
		switch {
		case errors.Is(err, partnersvc.ErrStoreNotConfigured):
			http.Redirect(w, r, "/setup", http.StatusFound)
			return
		case err != nil:
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		data := partnerProductNewPage(view.Categories)

		if r.Method == http.MethodGet {
			a.Render(w, a.Templates.PartnerProductNew, data)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		input := partnersvc.CreateProductInput{
			Name:        r.FormValue("name"),
			Description: r.FormValue("description"),
			Price:       r.FormValue("price"),
			Stock:       r.FormValue("stock_quantity"),
			CategoryID:  r.FormValue("category_id"),
		}
		data.DefaultName = strings.TrimSpace(input.Name)

		err = svc.CreateProduct(input)
		switch {
		case errors.Is(err, partnersvc.ErrStoreNotConfigured):
			http.Redirect(w, r, "/setup", http.StatusFound)
			return
		case errors.Is(err, partnersvc.ErrMissingProductFields):
			data.Error = "Name, price and stock are required"
			a.Render(w, a.Templates.PartnerProductNew, data)
			return
		case errors.Is(err, partnersvc.ErrInvalidProductPrice):
			data.Error = "Price must be a non-negative number"
			a.Render(w, a.Templates.PartnerProductNew, data)
			return
		case errors.Is(err, partnersvc.ErrInvalidProductStock):
			data.Error = "Stock must be a non-negative integer"
			a.Render(w, a.Templates.PartnerProductNew, data)
			return
		case errors.Is(err, partnersvc.ErrUnableToCreateProductSlug):
			data.Error = "Unable to create product slug"
			a.Render(w, a.Templates.PartnerProductNew, data)
			return
		case err != nil:
			data.Error = "Unable to create product"
			a.Render(w, a.Templates.PartnerProductNew, data)
			return
		}

		http.Redirect(w, r, "/products", http.StatusFound)
	}
}

package routes

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/app"
	partnersvc "github.com/kyambuthia/sokomoko/internal/service/partner"
)

type PartnerSetupData struct {
	Title     string
	Heading   string
	Message   string
	Error     string
	Settings  partnersvc.StoreSettings
	CSRFToken string
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
	CSRFToken        string
}

type PartnerProductsData struct {
	Title      string
	StoreName  string
	Products   []partnersvc.Product
	Message    string
	Error      string
	Categories []partnersvc.Category
	CSRFToken  string
}

type PartnerProductNewData struct {
	Title       string
	Message     string
	Error       string
	Categories  []partnersvc.Category
	Partners    []partnersvc.PartnerOption
	Form        partnersvc.CreateProductInput
	DefaultName string
	CSRFToken   string
}

func PartnerRoot(a *app.App) http.HandlerFunc {
	svc := a.Partner

	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			methodNotAllowed(w, http.MethodGet, http.MethodHead)
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
			Title:     "Store Setup",
			Heading:   "Partner Store Setup",
			CSRFToken: a.Auth.CSRFToken(r),
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
			a.RenderStatus(w, a.Templates.PartnerSetup, http.StatusBadRequest, data)
			return
		case errors.Is(err, partnersvc.ErrInvalidStoreSlug):
			data.Error = "Store slug may only contain lowercase letters, numbers, and single dashes."
			data.Settings = settings
			a.RenderStatus(w, a.Templates.PartnerSetup, http.StatusBadRequest, data)
			return
		case errors.Is(err, partnersvc.ErrInvalidContactEmail):
			data.Error = "Enter a valid contact email address."
			data.Settings = settings
			a.RenderStatus(w, a.Templates.PartnerSetup, http.StatusBadRequest, data)
			return
		case err != nil:
			log.Printf("partner setup: %v", err)
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
		page := partnerDashboardPage(view, role, username)
		page.CSRFToken = a.Auth.CSRFToken(r)
		a.Render(w, a.Templates.PartnerDashboard, page)
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

		page := partnerProductsPage(view)
		page.CSRFToken = a.Auth.CSRFToken(r)
		if flash := popFlash(w, r); flash.Message != "" {
			if flash.Kind == "danger" {
				page.Error = flash.Message
			} else {
				page.Message = flash.Message
			}
		}
		a.Render(w, a.Templates.PartnerProducts, page)
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

		data := partnerProductNewPage(view.Categories, a.Auth.CSRFToken(r))
		data.Partners = view.Partners

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
			PartnerID:   r.FormValue("partner_id"),
		}
		data.Form = input
		data.DefaultName = strings.TrimSpace(input.Name)

		err = svc.CreateProduct(input)
		switch {
		case errors.Is(err, partnersvc.ErrStoreNotConfigured):
			http.Redirect(w, r, "/setup", http.StatusFound)
			return
		case errors.Is(err, partnersvc.ErrMissingProductFields):
			data.Error = "Name, price and stock are required"
			a.RenderStatus(w, a.Templates.PartnerProductNew, http.StatusBadRequest, data)
			return
		case errors.Is(err, partnersvc.ErrInvalidProductPrice):
			data.Error = "Price must be an amount like 12.50 (up to 100000.00)"
			a.RenderStatus(w, a.Templates.PartnerProductNew, http.StatusBadRequest, data)
			return
		case errors.Is(err, partnersvc.ErrInvalidProductStock):
			data.Error = "Stock must be a non-negative integer"
			a.RenderStatus(w, a.Templates.PartnerProductNew, http.StatusBadRequest, data)
			return
		case errors.Is(err, partnersvc.ErrUnableToCreateProductSlug):
			data.Error = "Unable to create product slug"
			a.RenderStatus(w, a.Templates.PartnerProductNew, http.StatusBadRequest, data)
			return
		case err != nil:
			log.Printf("partner create product: %v", err)
			data.Error = "Unable to create product"
			a.RenderStatus(w, a.Templates.PartnerProductNew, http.StatusInternalServerError, data)
			return
		}

		redirectWithFlash(w, r, "/products", "success", "Product created")
	}
}

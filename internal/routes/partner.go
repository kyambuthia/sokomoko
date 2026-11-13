package routes

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/kyambuthia/sokomoko/internal/app"
	partnersvc "github.com/kyambuthia/sokomoko/internal/service/partner"
)

type PartnerSetupData struct {
	Role      string
	Title     string
	Heading   string
	Message   string
	Error     string
	Settings  partnersvc.StoreSettings
	CSRFToken string
	CanEdit   bool
}

type PartnerDashboardData struct {
	Title            string
	CSRFToken        string
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

		settings, err := svc.StoreSettings(r.Context())
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
		existing, err := svc.StoreSettings(r.Context())
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		data := PartnerSetupData{
			Title:     "Store Setup",
			Heading:   "Partner Store Setup",
			CSRFToken: a.Auth.CSRFToken(r),
			CanEdit:   partnersvc.CanEditStoreSettings(partnerActorFromContext(r)),
		}
		data.Role, _ = partnerIdentity(r)
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

		settings, err := svc.SaveStoreSettings(r.Context(), partnerActorFromContext(r), partnersvc.StoreSettingsInput{
			StoreName:    r.FormValue("store_name"),
			StoreSlug:    r.FormValue("store_slug"),
			Description:  r.FormValue("description"),
			ContactEmail: r.FormValue("contact_email"),
		})
		switch {
		case errors.Is(err, partnersvc.ErrForbiddenSettingsUpdate):
			data.Error = "Only platform admins and staff can change store settings."
			a.RenderStatus(w, a.Templates.PartnerSetup, http.StatusForbidden, data)
			return
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
			slog.ErrorContext(r.Context(), "partner setup", "error", err)
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
		view, err := svc.DashboardForActor(r.Context(), partnerActorFromContext(r))
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

package routes

import (
	"log/slog"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/kyambuthia/sokomoko/internal/app"
	"github.com/kyambuthia/sokomoko/internal/domain"
	catalogsvc "github.com/kyambuthia/sokomoko/internal/service/catalog"
	directorysvc "github.com/kyambuthia/sokomoko/internal/service/directory"
)

const (
	storeProductLimit = 48
	storeServiceLimit = 24
)

type StorePageData struct {
	Title        string
	Name         string
	Slug         string
	PartnerID    int
	Initials     string
	CanMessage   bool
	Products     []catalogsvc.Product
	Services     []directorysvc.ServiceListing
	ProductTotal int
	ServiceTotal int
	Viewer       Viewer
	CSRFToken    string
}

// StoreProfile serves /stores/{slug}, the public shop page.
func StoreProfile(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			methodNotAllowed(w, http.MethodGet, http.MethodHead)
			return
		}
		partner, err := a.Catalog.ActiveStore(r.Context(), r.PathValue("slug"))
		if err != nil {
			slog.ErrorContext(r.Context(), "store lookup failed", "request_id", app.RequestIDFromContext(r.Context()), "error", err)
			serverError(w, r)
			return
		}
		if partner == nil {
			NotFound(w, r)
			return
		}
		products, err := a.Catalog.Browse(r.Context(), catalogsvc.Filter{PartnerID: partner.ID, Limit: storeProductLimit})
		if err != nil {
			slog.ErrorContext(r.Context(), "store products failed", "request_id", app.RequestIDFromContext(r.Context()), "error", err)
			serverError(w, r)
			return
		}
		productTotal, err := a.Catalog.ProductCount(r.Context(), catalogsvc.Filter{PartnerID: partner.ID})
		if err != nil {
			slog.ErrorContext(r.Context(), "store product count failed", "request_id", app.RequestIDFromContext(r.Context()), "error", err)
			serverError(w, r)
			return
		}
		services, serviceTotal, err := a.Directory.Browse(r.Context(), directorysvc.ServiceFilter{PartnerID: partner.ID, Limit: storeServiceLimit}, 1)
		if err != nil {
			slog.ErrorContext(r.Context(), "store services failed", "request_id", app.RequestIDFromContext(r.Context()), "error", err)
			serverError(w, r)
			return
		}
		viewer := viewerFromRequest(r)
		a.Render(w, a.Templates.Store, StorePageData{
			Title:        partner.Name,
			Name:         partner.Name,
			Slug:         partner.Slug,
			PartnerID:    partner.ID,
			Initials:     storeInitials(partner.Name),
			CanMessage:   !viewer.SignedIn || viewer.Role == domain.RoleUser,
			Products:     products,
			Services:     services,
			ProductTotal: productTotal,
			ServiceTotal: serviceTotal,
			Viewer:       viewer,
			CSRFToken:    a.Auth.CSRFToken(r),
		})
	}
}

func storeInitials(name string) string {
	parts := strings.Fields(name)
	if len(parts) == 0 {
		return "S"
	}
	first, _ := utf8.DecodeRuneInString(parts[0])
	if first == utf8.RuneError {
		return "S"
	}
	if len(parts) == 1 {
		return strings.ToUpper(string(first))
	}
	second, _ := utf8.DecodeRuneInString(parts[1])
	if second == utf8.RuneError {
		return strings.ToUpper(string(first))
	}
	return strings.ToUpper(string([]rune{first, second}))
}

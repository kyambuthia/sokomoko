package routes

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/kyambuthia/sokomoko/internal/app"
	"github.com/kyambuthia/sokomoko/internal/auth"
	"github.com/kyambuthia/sokomoko/internal/db"
	directorysvc "github.com/kyambuthia/sokomoko/internal/service/directory"
)

type servicePage struct {
	Title, Role, CSRFToken, Error string
	Viewer                        Viewer
	Listings                      []db.ServiceListing
	Listing                       *db.ServiceListing
	Filter                        db.ServiceFilter
	Pager                         Pager
	Form                          directorysvc.Input
	EditID                        int
	Categories                    []string
}

var serviceCategories = []string{"hair", "beauty", "dental", "wellness", "home", "other"}

func directoryActor(r *http.Request) *directorysvc.Actor {
	u := auth.GetUserFromContext(r.Context())
	if u == nil {
		return nil
	}
	return &directorysvc.Actor{ID: u.ID, Role: u.Role}
}
func servicePageError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, db.ErrServiceNotFound):
		NotFound(w, r)
	case errors.Is(err, directorysvc.ErrForbidden):
		http.Error(w, "Seller account required", http.StatusForbidden)
	case errors.Is(err, directorysvc.ErrInvalid):
		http.Error(w, "Invalid service filters or fields", http.StatusBadRequest)
	default:
		serverError(w, r)
	}
}
func Services(a *app.App, own bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			methodNotAllowed(w, http.MethodGet, http.MethodHead)
			return
		}
		page := pageParam(r)
		data := servicePage{Title: "Services", Viewer: viewerFromRequest(r), CSRFToken: a.Auth.CSRFToken(r), Categories: serviceCategories}
		data.Filter = db.ServiceFilter{Query: r.URL.Query().Get("q"), Category: r.URL.Query().Get("category"), Area: r.URL.Query().Get("area"), Limit: 24}
		var total int
		var err error
		if own {
			data.Role = db.RolePartner
			data.Listings, total, err = a.Directory.Own(r.Context(), directoryActor(r), page, 24)
		} else {
			data.Listings, total, err = a.Directory.Browse(r.Context(), data.Filter, page)
		}
		if err != nil {
			servicePageError(w, r, err)
			return
		}
		if redirectPastLastPage(w, r, page, 24, total) {
			return
		}
		data.Pager = newPager(r, page, 24, total)
		if own {
			a.Render(w, a.Templates.PartnerServices, data)
		} else {
			a.Render(w, a.Templates.Services, data)
		}
	}
}
func ServiceDetail(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			methodNotAllowed(w, http.MethodGet, http.MethodHead)
			return
		}
		listing, err := a.Directory.Public(r.Context(), r.PathValue("slug"))
		if err != nil {
			servicePageError(w, r, err)
			return
		}
		a.Render(w, a.Templates.ServiceDetail, servicePage{Title: listing.Title, Listing: listing, Viewer: viewerFromRequest(r), CSRFToken: a.Auth.CSRFToken(r)})
	}
}
func PartnerServiceEdit(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodGet, http.MethodPost)
			return
		}
		id := 0
		if key := r.PathValue("id"); key != "" {
			var err error
			id, err = strconv.Atoi(key)
			if err != nil || id < 1 {
				NotFound(w, r)
				return
			}
		}
		data := servicePage{Title: "Services", Role: db.RolePartner, CSRFToken: a.Auth.CSRFToken(r), EditID: id, Categories: serviceCategories}
		actor := directoryActor(r)
		if r.Method == http.MethodGet {
			data.Form = directorysvc.Input{Category: "hair", DurationMinutes: 30, PriceMode: "fixed", Price: "0.00", Currency: "KES", DeliveryMode: "premises", State: "draft"}
			if id > 0 {
				v, err := a.Directory.Edit(r.Context(), actor, id)
				if err != nil {
					servicePageError(w, r, err)
					return
				}
				data.Form = directorysvc.Input{Title: v.Title, Description: v.Description, Category: v.Category, DurationMinutes: v.DurationMinutes, PriceMode: v.PriceMode, Price: v.PriceCents.String(), Currency: v.Currency, PublicArea: v.PublicArea, DeliveryMode: v.DeliveryMode, State: v.State, Version: v.Version}
			}
			a.Render(w, a.Templates.PartnerServiceEdit, data)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid form", http.StatusBadRequest)
			return
		}
		duration, durationErr := strconv.Atoi(r.PostForm.Get("duration_minutes"))
		version, versionErr := strconv.Atoi(r.PostForm.Get("version"))
		data.Form = directorysvc.Input{Title: r.PostForm.Get("title"), Description: r.PostForm.Get("description"), Category: r.PostForm.Get("category"), DurationMinutes: duration, PriceMode: r.PostForm.Get("price_mode"), Price: r.PostForm.Get("price"), Currency: r.PostForm.Get("currency"), PublicArea: r.PostForm.Get("public_area"), DeliveryMode: r.PostForm.Get("delivery_mode"), State: r.PostForm.Get("state"), Version: version}
		var err error
		if durationErr != nil || versionErr != nil {
			err = directorysvc.ErrInvalid
		} else {
			_, err = a.Directory.Save(r.Context(), actor, id, data.Form)
		}
		if err == nil {
			http.Redirect(w, r, "/services", http.StatusSeeOther)
			return
		}
		status := http.StatusBadRequest
		data.Error = "Check all fields: title (3–120 characters), description (10–3000), duration (5–480 minutes), price and public area (2–120). Quote-only prices must be zero."
		if errors.Is(err, db.ErrServiceConflict) {
			status = http.StatusConflict
			data.Error = "Someone edited this listing. Your text is kept below. Open the current listing in another tab and review it before saving again."
		} else if !errors.Is(err, directorysvc.ErrInvalid) {
			servicePageError(w, r, err)
			return
		}
		a.RenderStatus(w, a.Templates.PartnerServiceEdit, status, data)
	}
}

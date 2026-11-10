package routes

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/kyambuthia/sokomoko/internal/app"
	"github.com/kyambuthia/sokomoko/internal/auth"
	"github.com/kyambuthia/sokomoko/internal/db"
	bookingsvc "github.com/kyambuthia/sokomoko/internal/service/booking"
	directorysvc "github.com/kyambuthia/sokomoko/internal/service/directory"
)

type serviceWeekday struct {
	Number   int
	Name     string
	OpensAt  string
	ClosesAt string
}

// serviceCategoryLink is one quick-filter chip on the public directory. Href
// keeps the visitor's other filters and resets pagination.
type serviceCategoryLink struct {
	Name    string
	Href    string
	Current bool
}

type servicePage struct {
	ConflictListing               *db.ServiceListing
	State                         string
	Preview                       bool
	Total                         int
	CategoryLinks                 []serviceCategoryLink
	Week                          []serviceWeekday
	Today                         int
	FieldErrors                   map[string]string
	ValidationFields              []directorysvc.FieldError
	Duration                      string
	Title, Role, CSRFToken, Error string
	BookingKey                    string
	LocalDate                     string
	Flash                         Flash
	Viewer                        Viewer
	Listings                      []db.ServiceListing
	Listing                       *db.ServiceListing
	Filter                        db.ServiceFilter
	Pager                         Pager
	Form                          directorysvc.Input
	EditID                        int
	Categories                    []string
	Weekdays                      []serviceWeekday
	Schedule                      []serviceWeekday
	ConflictSchedule              []serviceWeekday
}

var serviceCategories = []string{"hair", "beauty", "dental", "wellness", "home", "other"}
var serviceWeekdayNames = [...]string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}

func serviceWeekdayForms(hours []db.ServiceWeeklyHour) []serviceWeekday {
	byDay := make(map[int]db.ServiceWeeklyHour, len(hours))
	for _, hour := range hours {
		if hour.Weekday >= 0 && hour.Weekday < len(serviceWeekdayNames) {
			byDay[hour.Weekday] = hour
		}
	}
	days := make([]serviceWeekday, 0, len(serviceWeekdayNames))
	for number, name := range serviceWeekdayNames {
		hour := byDay[number]
		days = append(days, serviceWeekday{Number: number, Name: name, OpensAt: hour.OpensAt, ClosesAt: hour.ClosesAt})
	}
	return days
}

func serviceSchedule(hours []db.ServiceWeeklyHour) []serviceWeekday {
	days := serviceWeekdayForms(hours)
	schedule := make([]serviceWeekday, 0, len(hours))
	for _, day := range days {
		if day.OpensAt != "" && day.ClosesAt != "" {
			schedule = append(schedule, day)
		}
	}
	return schedule
}

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
		page := pageParamBounded(r, 1000)
		data := servicePage{Title: "Services", Viewer: viewerFromRequest(r), CSRFToken: a.Auth.CSRFToken(r), Categories: serviceCategories}
		data.Filter = db.ServiceFilter{Query: r.URL.Query().Get("q"), Category: r.URL.Query().Get("category"), Area: r.URL.Query().Get("area"), DeliveryMode: r.URL.Query().Get("delivery_mode"), Limit: 24}
		var total int
		var err error
		if own {
			data.Role = db.RolePartner
			data.State = r.URL.Query().Get("state")
			data.Listings, total, err = a.Directory.OwnByState(r.Context(), directoryActor(r), data.State, page, 24)
		} else {
			if raw := r.URL.Query().Get("has_hours"); raw != "" {
				data.Filter.HasHours, err = strconv.ParseBool(raw)
				if err != nil {
					servicePageError(w, r, directorysvc.ErrInvalid)
					return
				}
			}
			if raw := r.URL.Query().Get("partner_id"); raw != "" {
				data.Filter.PartnerID, err = strconv.Atoi(raw)
				if err != nil || data.Filter.PartnerID <= 0 {
					servicePageError(w, r, directorysvc.ErrInvalid)
					return
				}
			}
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
		data.Total = total
		if !own {
			data.CategoryLinks = serviceCategoryLinks(r.URL.Query(), data.Filter.Category)
		}
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
		data := servicePage{
			Title: listing.Title, Listing: listing, Schedule: serviceSchedule(listing.WeeklyHours),
			Week: serviceWeekdayForms(listing.WeeklyHours), Today: serviceLocalWeekday(listing.Timezone),
			Viewer: viewerFromRequest(r), CSRFToken: a.Auth.CSRFToken(r), Flash: popFlash(w, r),
			LocalDate: serviceLocalDate(listing.Timezone),
		}
		if user := requestUserFromContext(r); user != nil && user.Role == db.RoleUser {
			data.BookingKey, err = newBookingIdempotencyKey()
			if err != nil {
				http.Error(w, "Could not prepare appointment form", http.StatusInternalServerError)
				return
			}
		}
		a.Render(w, a.Templates.ServiceDetail, data)
	}
}

func serviceLocalDate(timezone string) string {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		loc = time.UTC
	}
	return time.Now().In(loc).Format("2006-01-02")
}

// serviceLocalWeekday is today's weekday (Sunday=0) in the listing's
// timezone, so the weekly hours table can mark the current day.
func serviceLocalWeekday(timezone string) int {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		loc = time.UTC
	}
	return int(time.Now().In(loc).Weekday())
}

// serviceCategoryLinks builds the directory's category chips: "All" first,
// then each category. Every link keeps the other active filters and drops
// the page number, because the result set changes.
func serviceCategoryLinks(query url.Values, current string) []serviceCategoryLink {
	link := func(category string) string {
		q := url.Values{}
		for key, values := range query {
			if key != "page" && key != "category" {
				q[key] = values
			}
		}
		if category != "" {
			q.Set("category", category)
		}
		if encoded := q.Encode(); encoded != "" {
			return "/services?" + encoded
		}
		return "/services"
	}
	links := make([]serviceCategoryLink, 0, len(serviceCategories)+1)
	links = append(links, serviceCategoryLink{Name: "all", Href: link(""), Current: current == ""})
	for _, category := range serviceCategories {
		links = append(links, serviceCategoryLink{Name: category, Href: link(category), Current: category == current})
	}
	return links
}

func newBookingIdempotencyKey() (string, error) {
	var key [32]byte
	if _, err := rand.Read(key[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(key[:]), nil
}

// ServiceBookingRequest creates a pending request for a service appointment.
func ServiceBookingRequest(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		if a.DemoMode {
			redirectWithFlash(w, r, "/services", "danger", "Appointment requests are disabled in demo mode.")
			return
		}
		listingSlug := r.PathValue("slug")
		if strings.TrimSpace(listingSlug) == "" {
			NotFound(w, r)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid form", http.StatusBadRequest)
			return
		}
		version, err := strconv.Atoi(strings.TrimSpace(r.FormValue("listing_version")))
		if err != nil || version < 1 {
			redirectWithFlash(w, r, "/services/"+url.PathEscape(listingSlug), "danger", "Reload this service page before requesting an appointment.")
			return
		}
		user := requestUserFromContext(r)
		if user == nil || user.Role != db.RoleUser {
			http.Error(w, "Customer sign-in required", http.StatusForbidden)
			return
		}
		booking, err := a.Bookings.Request(
			r.Context(), &bookingsvc.Actor{ID: user.ID, Role: user.Role}, listingSlug,
			r.FormValue("starts_at_local"), version, r.FormValue("idempotency_key"),
		)
		if err != nil {
			message := ""
			switch {
			case errors.Is(err, bookingsvc.ErrInvalid):
				message = "Choose a valid future date and time, and submit the form again."
			case errors.Is(err, bookingsvc.ErrListingChanged):
				message = "This service changed. Reload its page before requesting an appointment."
			case errors.Is(err, bookingsvc.ErrTimeConflict):
				message = "That time has already been requested. Choose another time."
			case errors.Is(err, bookingsvc.ErrUnavailable), errors.Is(err, db.ErrServiceNotFound):
				message = "That service is not available at the selected time."
			case errors.Is(err, bookingsvc.ErrIdempotencyConflict):
				message = "This request has already been used. Reload the service page and try again."
			default:
				servicePageError(w, r, err)
				return
			}
			redirectWithFlash(w, r, "/services/"+url.PathEscape(listingSlug), "danger", message)
			return
		}
		message := "Appointment request sent"
		if booking != nil && booking.ID > 0 {
			message += " (#" + strconv.FormatInt(booking.ID, 10) + ")"
		}
		message += ". The provider has up to 24 hours to respond."
		redirectWithFlash(w, r, "/services/"+url.PathEscape(listingSlug), "success", message)
	}
}

// PartnerServicePreview renders saved listings without exposing private states publicly.
func PartnerServicePreview(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			methodNotAllowed(w, http.MethodGet, http.MethodHead)
			return
		}
		id, err := strconv.Atoi(r.PathValue("id"))
		if err != nil || id < 1 {
			NotFound(w, r)
			return
		}
		listing, err := a.Directory.Edit(r.Context(), directoryActor(r), id)
		if err != nil {
			servicePageError(w, r, err)
			return
		}
		a.Render(w, a.Templates.ServiceDetail, servicePage{Preview: true, Title: listing.Title, Role: db.RolePartner, Listing: listing, Schedule: serviceSchedule(listing.WeeklyHours), Viewer: viewerFromRequest(r), CSRFToken: a.Auth.CSRFToken(r)})
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
			data.Form = directorysvc.Input{Category: "hair", DurationMinutes: 30, PriceMode: "fixed", Price: "0.00", Currency: "KES", DeliveryMode: "premises", State: "draft", Timezone: "Africa/Nairobi"}
			if id > 0 {
				v, err := a.Directory.Edit(r.Context(), actor, id)
				if err != nil {
					servicePageError(w, r, err)
					return
				}
				data.Form = directorysvc.Input{Title: v.Title, Description: v.Description, Category: v.Category, DurationMinutes: v.DurationMinutes, PriceMode: v.PriceMode, Price: v.PriceCents.String(), Currency: v.Currency, PublicArea: v.PublicArea, DeliveryMode: v.DeliveryMode, State: v.State, Version: v.Version, Timezone: v.Timezone, WeeklyHours: v.WeeklyHours}
			}
			data.Duration = strconv.Itoa(data.Form.DurationMinutes)
			data.Weekdays = serviceWeekdayForms(data.Form.WeeklyHours)
			a.Render(w, a.Templates.PartnerServiceEdit, data)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid form", http.StatusBadRequest)
			return
		}
		data.Duration = r.PostForm.Get("duration_minutes")
		duration, durationErr := strconv.Atoi(data.Duration)
		if durationErr != nil {
			duration = 0
		}
		version, versionErr := strconv.Atoi(r.PostForm.Get("version"))
		data.Form = directorysvc.Input{Title: r.PostForm.Get("title"), Description: r.PostForm.Get("description"), Category: r.PostForm.Get("category"), DurationMinutes: duration, PriceMode: r.PostForm.Get("price_mode"), Price: r.PostForm.Get("price"), Currency: r.PostForm.Get("currency"), PublicArea: r.PostForm.Get("public_area"), DeliveryMode: r.PostForm.Get("delivery_mode"), State: r.PostForm.Get("state"), Version: version, Timezone: r.PostForm.Get("timezone"), WeeklyHours: []db.ServiceWeeklyHour{}}
		for weekday := range serviceWeekdayNames {
			opensAt := strings.TrimSpace(r.PostForm.Get("opens_at_" + strconv.Itoa(weekday)))
			closesAt := strings.TrimSpace(r.PostForm.Get("closes_at_" + strconv.Itoa(weekday)))
			if opensAt == "" && closesAt == "" {
				continue
			}
			data.Form.WeeklyHours = append(data.Form.WeeklyHours, db.ServiceWeeklyHour{Weekday: weekday, OpensAt: opensAt, ClosesAt: closesAt})
		}
		data.Weekdays = serviceWeekdayForms(data.Form.WeeklyHours)
		if versionErr != nil {
			data.Form.Version = -1
		}
		// A conflict retry requires an explicit acknowledgement of the displayed
		// saved version. The store still rejects a newer intervening revision.
		if reviewed := r.PostForm.Get("reviewed_version"); reviewed != "" {
			reviewedVersion, err := strconv.Atoi(reviewed)
			if err != nil || id == 0 || versionErr != nil || reviewedVersion <= version {
				data.Form.Version = -1
			} else {
				data.Form.Version = reviewedVersion
			}
		}
		_, err := a.Directory.Save(r.Context(), actor, id, data.Form)
		if err == nil {
			http.Redirect(w, r, "/services", http.StatusSeeOther)
			return
		}
		status := http.StatusBadRequest
		data.Error = "Check the highlighted fields and save again."
		var validation *directorysvc.ValidationError
		if errors.As(err, &validation) {
			data.ValidationFields = validation.Fields
			data.FieldErrors = make(map[string]string, len(validation.Fields))
			for _, field := range validation.Fields {
				data.FieldErrors[field.Field] = field.Message
			}
		}
		if errors.Is(err, db.ErrServiceConflict) {
			status = http.StatusConflict
			data.ConflictListing, err = a.Directory.Edit(r.Context(), actor, id)
			if err != nil {
				servicePageError(w, r, err)
				return
			}
			data.ConflictSchedule = serviceSchedule(data.ConflictListing.WeeklyHours)
			data.Error = "Someone edited this listing. Your text is kept below. Review the current saved version before applying your changes."
		} else if !errors.Is(err, directorysvc.ErrInvalid) {
			servicePageError(w, r, err)
			return
		}
		a.RenderStatus(w, a.Templates.PartnerServiceEdit, status, data)
	}
}

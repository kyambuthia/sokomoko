package routes

import (
	"errors"
	"net/http"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/app"
	"github.com/kyambuthia/sokomoko/internal/auth"
	"github.com/kyambuthia/sokomoko/internal/domain"
	logisticsvc "github.com/kyambuthia/sokomoko/internal/service/logistics"
)

type CourierDashboardData struct {
	View       string
	Title      string
	Username   string
	CSRFToken  string
	Message    string
	Error      string
	Deliveries []logisticsvc.CourierDelivery
}

func RegisterCourier(a *app.App, mux *http.ServeMux, trustProxyHeaders bool) {
	mux.HandleFunc("/healthz", Health())
	mux.HandleFunc("/readyz", Ready(a))
	mux.Handle("/static/", Static(a.StaticFS))
	mux.HandleFunc("/storefront", redirectToSiblingWithProxyTrust("", "/", trustProxyHeaders))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			methodNotAllowed(w, http.MethodGet, http.MethodHead)
			return
		}
		http.Redirect(w, r, "/deliveries", http.StatusFound)
	})
	mux.HandleFunc("/login", a.Auth.CourierLogin(a.Templates.AdminLogin))
	mux.HandleFunc("/logout", a.Auth.Logout())
	mux.Handle("/password", withRole(a, domain.RoleCourier, a.Auth.PasswordChange(a.Templates.PasswordChange, "/password", "/deliveries")))
	mux.Handle("/deliveries", withRole(a, domain.RoleCourier, CourierDeliveries(a)))
	mux.Handle("/deliveries/", withRole(a, domain.RoleCourier, CourierDeliveryUpdate(a)))
	mux.Handle("/password-reset/", http.NotFoundHandler())
	registerMessaging(a, mux, "courier", []string{domain.RoleCourier})
}

func CourierDeliveries(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		view := r.URL.Query().Get("view")
		if view == "" {
			view = "all"
		}
		jobs, err := a.Logistics.JobsByView(r.Context(), logisticsActorFromContext(r), view)
		if errors.Is(err, logisticsvc.ErrInvalidCourierView) {
			http.Error(w, "Invalid delivery view", http.StatusBadRequest)
			return
		}
		if err != nil {
			http.Error(w, "Unable to load deliveries", http.StatusInternalServerError)
			return
		}
		user := auth.GetUserFromContext(r.Context())
		data := CourierDashboardData{
			View: view, Title: "My Deliveries", CSRFToken: a.Auth.CSRFToken(r), Deliveries: jobs,
		}
		if user != nil {
			data.Username = user.Username
		}
		if flash := popFlash(w, r); flash.Message != "" {
			if flash.Kind == "danger" {
				data.Error = flash.Message
			} else {
				data.Message = flash.Message
			}
		}
		a.Render(w, a.Templates.CourierDashboard, data)
	}
}

func CourierDeliveryUpdate(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		destination := "/deliveries"
		if view := r.FormValue("view"); view == "active" || view == "completed" {
			destination += "?view=" + view
		}
		id := r.PathValue("id")
		if id == "" {
			id = strings.TrimPrefix(r.URL.Path, "/deliveries/")
		}
		err := a.Logistics.Update(r.Context(), logisticsActorFromContext(r), id, r.FormValue("status"), r.FormValue("note"))
		switch {
		case err == nil:
			redirectWithFlash(w, r, destination, "success", "Delivery status updated")
		case errors.Is(err, logisticsvc.ErrForbidden):
			http.Error(w, "Forbidden", http.StatusForbidden)
		case errors.Is(err, logisticsvc.ErrDeliveryNotFound):
			http.Error(w, "Delivery not found", http.StatusNotFound)
		case errors.Is(err, logisticsvc.ErrInvalidDeliveryID):
			http.Error(w, "Invalid delivery id", http.StatusBadRequest)
		case errors.Is(err, logisticsvc.ErrFailureNoteRequired):
			redirectWithFlash(w, r, destination, "danger", "Choose a reason before reporting a failed delivery")
		case errors.Is(err, logisticsvc.ErrInvalidTransition):
			redirectWithFlash(w, r, destination, "danger", "That delivery update is not allowed")
		default:
			http.Error(w, "Unable to update delivery", http.StatusInternalServerError)
		}
	}
}

func logisticsActorFromContext(r *http.Request) *logisticsvc.Actor {
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		return nil
	}
	return &logisticsvc.Actor{ID: user.ID, Role: user.Role}
}

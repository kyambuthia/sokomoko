package routes

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kyambuthia/sokomoko/internal/app"
	bookingsvc "github.com/kyambuthia/sokomoko/internal/service/booking"
)

const serviceBookingsPageSize = 20

type serviceBookingCard struct {
	Booking           bookingsvc.ServiceBooking
	StatusLabel       string
	StatusDescription string
	LocalStart        string
	LocalDeadline     string
	CanStart          bool
	CanCancel         bool
	CanComplete       bool
	CanMarkNoShow     bool
}

type serviceBookingsPage struct {
	Title     string
	Role      string
	CSRFToken string
	Flash     Flash
	Bookings  []serviceBookingCard
	Pager     Pager
}

func serviceBookingCardFor(booking bookingsvc.ServiceBooking) serviceBookingCard {
	loc, err := time.LoadLocation(booking.Timezone)
	if err != nil {
		loc = time.UTC
	}
	now := time.Now()
	return serviceBookingCard{
		Booking:           booking,
		StatusLabel:       bookingStatusLabel(booking.Status),
		StatusDescription: serviceBookingStatusDescription(booking.Status),
		LocalStart:        booking.StartsAt.In(loc).Format("Mon 2 Jan 2006, 15:04 MST"),
		LocalDeadline:     booking.ResponseDeadline.In(loc).Format("Mon 2 Jan 2006, 15:04 MST"),
		CanStart:          booking.Status == "confirmed" && !booking.StartsAt.After(now) && booking.EndsAt.After(now),
		CanCancel:         booking.Status == "confirmed" && booking.StartsAt.After(now),
		CanComplete:       booking.Status == "in_progress" && !booking.EndsAt.After(now),
		CanMarkNoShow:     booking.Status == "confirmed" && !booking.EndsAt.After(now),
	}
}

func bookingStatusLabel(status string) string {
	switch status {
	case "in_progress":
		return "In progress"
	case "no_show":
		return "No-show"
	default:
		if status == "" {
			return "Unknown"
		}
		return strings.ToUpper(status[:1]) + strings.ReplaceAll(status[1:], "_", " ")
	}
}

func PartnerServiceBookings(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			methodNotAllowed(w, http.MethodGet, http.MethodHead)
			return
		}
		user := requestUserFromContext(r)
		if user == nil {
			http.Error(w, "Authentication required", http.StatusUnauthorized)
			return
		}
		page := pageParamBounded(r, 1000)
		bookings, total, err := a.Bookings.ForPartner(
			r.Context(), &bookingsvc.Actor{ID: user.ID, Role: user.Role}, page, serviceBookingsPageSize,
		)
		if err != nil {
			if errors.Is(err, bookingsvc.ErrForbidden) {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			slog.ErrorContext(r.Context(), "partner service bookings", "error", err)
			serverError(w, r)
			return
		}
		if redirectPastLastPage(w, r, page, serviceBookingsPageSize, total) {
			return
		}
		cards := make([]serviceBookingCard, 0, len(bookings))
		for _, booking := range bookings {
			cards = append(cards, serviceBookingCardFor(booking))
		}
		a.Render(w, a.Templates.PartnerBookings, serviceBookingsPage{
			Title: "Appointment Requests", Role: user.Role, CSRFToken: a.Auth.CSRFToken(r),
			Flash: popFlash(w, r), Bookings: cards,
			Pager: newPager(r, page, serviceBookingsPageSize, total),
		})
	}
}

func PartnerServiceBookingAction(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		id, err := strconv.Atoi(r.PathValue("id"))
		if err != nil || id <= 0 {
			http.NotFound(w, r)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid form", http.StatusBadRequest)
			return
		}
		version, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("version")), 10, 64)
		if err != nil || version <= 0 {
			redirectWithFlash(w, r, "/service-bookings", "danger", "Reload the appointment queue before updating a request.")
			return
		}
		user := requestUserFromContext(r)
		if user == nil {
			http.Error(w, "Authentication required", http.StatusUnauthorized)
			return
		}
		action := r.FormValue("action")
		err = a.Bookings.Respond(r.Context(), &bookingsvc.Actor{ID: user.ID, Role: user.Role}, id, version, action)
		switch {
		case err == nil:
			message := "Appointment request updated."
			switch action {
			case "confirm":
				message = "Appointment confirmed."
			case "reject":
				message = "Appointment request declined."
			case "cancel":
				message = "Appointment cancelled."
			case "start":
				message = "Appointment marked in progress."
			case "complete":
				message = "Appointment marked complete."
			case "no_show":
				message = "No-show recorded."
			}
			redirectWithFlash(w, r, "/service-bookings", "success", message)
		case errors.Is(err, bookingsvc.ErrInvalid):
			redirectWithFlash(w, r, "/service-bookings", "danger", "Choose a valid appointment action.")
		case errors.Is(err, bookingsvc.ErrNotFound):
			http.NotFound(w, r)
		case errors.Is(err, bookingsvc.ErrForbidden):
			http.Error(w, "Forbidden", http.StatusForbidden)
		case errors.Is(err, bookingsvc.ErrResponseExpired):
			redirectWithFlash(w, r, "/service-bookings", "danger", "The response deadline passed. This request is no longer active.")
		case errors.Is(err, bookingsvc.ErrVersionConflict):
			redirectWithFlash(w, r, "/service-bookings", "danger", "This appointment changed. Reload the queue before updating it.")
		case errors.Is(err, bookingsvc.ErrInvalidState):
			redirectWithFlash(w, r, "/service-bookings", "danger", "This appointment can no longer be updated with that action.")
		default:
			slog.ErrorContext(r.Context(), "update service booking", "error", err)
			serverError(w, r)
		}
	}
}

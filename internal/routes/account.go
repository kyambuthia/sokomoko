package routes

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kyambuthia/sokomoko/internal/app"
	"github.com/kyambuthia/sokomoko/internal/db"
	accountsvc "github.com/kyambuthia/sokomoko/internal/service/account"
	bookingsvc "github.com/kyambuthia/sokomoko/internal/service/booking"
)

// accountOrdersPerPage is how many orders the account page shows at once.
const accountOrdersPerPage = 20

type AccountPageData struct {
	Title     string
	Flash     Flash
	Orders    []accountsvc.Order
	Bookings  []AccountBookingView
	Pager     Pager
	Viewer    Viewer
	CSRFToken string
}

type AccountBookingView struct {
	Booking           db.ServiceBooking
	StatusLabel       string
	StatusDescription string
	LocalStart        string
	LocalDeadline     string
	CanCancel         bool
}

type AccountServiceBookingsPage struct {
	Title     string
	Flash     Flash
	Bookings  []AccountBookingView
	Pager     Pager
	Viewer    Viewer
	CSRFToken string
}

func accountBookingView(booking db.ServiceBooking) AccountBookingView {
	loc, err := time.LoadLocation(booking.Timezone)
	if err != nil {
		loc = time.UTC
	}
	return AccountBookingView{
		Booking: booking, StatusLabel: bookingStatusLabel(booking.Status),
		StatusDescription: serviceBookingStatusDescription(booking.Status),
		LocalStart:        booking.StartsAt.In(loc).Format("Mon 2 Jan 2006, 15:04 MST"),
		LocalDeadline:     booking.ResponseDeadline.In(loc).Format("Mon 2 Jan 2006, 15:04 MST"),
		CanCancel:         (booking.Status == "requested" || booking.Status == "confirmed") && booking.StartsAt.After(time.Now()),
	}
}

func serviceBookingStatusDescription(status string) string {
	switch status {
	case "rejected":
		return "This request was declined."
	case "expired":
		return "The response deadline passed before anyone responded."
	case "cancelled":
		return "This appointment was cancelled."
	case "in_progress":
		return "The appointment is in progress."
	case "completed":
		return "The appointment is complete."
	case "no_show":
		return "This appointment was marked as a no-show."
	default:
		return ""
	}
}

// Account serves the customer's order history.
func Account(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			methodNotAllowed(w, http.MethodGet, http.MethodHead)
			return
		}
		userID, _ := requestUserID(r)
		page := pageParam(r)
		var bookingViews []AccountBookingView
		if user := requestUserFromContext(r); user != nil && user.Role == db.RoleUser && a.Bookings != nil {
			bookings, _, bookingErr := a.Bookings.ForCustomer(r.Context(), &bookingsvc.Actor{ID: user.ID, Role: user.Role}, 1, 10)
			if bookingErr != nil {
				log.Printf("account service bookings: %v", bookingErr)
				serverError(w, r)
				return
			}
			bookingViews = make([]AccountBookingView, 0, len(bookings))
			for _, booking := range bookings {
				bookingViews = append(bookingViews, accountBookingView(booking))
			}
		}
		orders, total, err := a.Account.OrdersForUserPage(r.Context(), userID, page, accountOrdersPerPage)
		if err != nil {
			log.Printf("account orders: %v", err)
			serverError(w, r)
			return
		}
		if redirectPastLastPage(w, r, page, accountOrdersPerPage, total) {
			return
		}
		a.Render(w, a.Templates.Account, AccountPageData{
			Title:     "Your Account",
			Flash:     popFlash(w, r),
			Orders:    orders,
			Bookings:  bookingViews,
			Pager:     newPager(r, page, accountOrdersPerPage, total),
			Viewer:    viewerFromRequest(r),
			CSRFToken: a.Auth.CSRFToken(r),
		})
	}
}

// AccountServiceBookings lists the customer's complete appointment history.
func AccountServiceBookings(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			methodNotAllowed(w, http.MethodGet, http.MethodHead)
			return
		}
		user := requestUserFromContext(r)
		if user == nil || user.Role != db.RoleUser {
			http.Error(w, "Customer sign-in required", http.StatusUnauthorized)
			return
		}
		page := serviceBookingsPageParam(r)
		bookings, total, err := a.Bookings.ForCustomer(
			r.Context(), &bookingsvc.Actor{ID: user.ID, Role: user.Role}, page, serviceBookingsPageSize,
		)
		if err != nil {
			log.Printf("account service bookings: %v", err)
			serverError(w, r)
			return
		}
		if redirectPastLastPage(w, r, page, serviceBookingsPageSize, total) {
			return
		}
		views := make([]AccountBookingView, 0, len(bookings))
		for _, booking := range bookings {
			views = append(views, accountBookingView(booking))
		}
		a.Render(w, a.Templates.AccountBookings, AccountServiceBookingsPage{
			Title: "Your Appointments", Flash: popFlash(w, r), Bookings: views,
			Pager: newPager(r, page, serviceBookingsPageSize, total), Viewer: viewerFromRequest(r),
			CSRFToken: a.Auth.CSRFToken(r),
		})
	}
}

// AccountOrderCancel cancels one of the customer's orders before the vendor
// accepts it and before it is paid.
func AccountOrderCancel(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		userID, _ := requestUserID(r)
		orderID, err := strconv.Atoi(r.PathValue("id"))
		if err != nil || orderID <= 0 {
			http.NotFound(w, r)
			return
		}
		err = a.Account.CancelOrder(r.Context(), userID, orderID)
		switch {
		case err == nil:
			redirectWithFlash(w, r, "/account", "success", fmt.Sprintf("Order #%d cancelled", orderID))
		case errors.Is(err, accountsvc.ErrOrderNotFound):
			http.NotFound(w, r)
		case errors.Is(err, accountsvc.ErrOrderNotCancellable):
			redirectWithFlash(w, r, "/account", "danger", fmt.Sprintf("Order #%d is already being prepared or paid. Contact the store to cancel it.", orderID))
		default:
			log.Printf("cancel order: %v", err)
			serverError(w, r)
		}
	}
}

// AccountBookingCancel cancels a customer's own future service appointment.
func AccountBookingCancel(a *app.App) http.HandlerFunc {
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
			redirectWithFlash(w, r, "/account", "danger", "Reload your account before cancelling the appointment.")
			return
		}
		user := requestUserFromContext(r)
		if user == nil {
			http.Error(w, "Authentication required", http.StatusUnauthorized)
			return
		}
		err = a.Bookings.Cancel(r.Context(), &bookingsvc.Actor{ID: user.ID, Role: user.Role}, id, version)
		switch {
		case err == nil:
			redirectWithFlash(w, r, "/account", "success", "Appointment request cancelled.")
		case errors.Is(err, bookingsvc.ErrNotFound):
			http.NotFound(w, r)
		case errors.Is(err, bookingsvc.ErrVersionConflict):
			redirectWithFlash(w, r, "/account", "danger", "This appointment changed. Reload the page before cancelling it.")
		case errors.Is(err, bookingsvc.ErrInvalidState):
			redirectWithFlash(w, r, "/account", "danger", "This appointment can no longer be cancelled here.")
		case errors.Is(err, bookingsvc.ErrForbidden):
			http.Error(w, "Forbidden", http.StatusForbidden)
		default:
			log.Printf("cancel service booking: %v", err)
			serverError(w, r)
		}
	}
}

package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/auth"
	"github.com/kyambuthia/sokomoko/internal/db"
	bookingsvc "github.com/kyambuthia/sokomoko/internal/service/booking"
)

func (h *Handler) createServiceBooking(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if h.app.DemoMode {
		writeError(w, http.StatusForbidden, "demo_mode", "This demo does not accept appointment requests")
		return
	}
	user := auth.GetUserFromContext(r.Context())
	if user == nil || user.Role != db.RoleUser {
		writeError(w, http.StatusForbidden, "forbidden", "A customer account is required")
		return
	}
	var fields map[string]json.RawMessage
	if err := decodeJSON(r, &fields); err != nil || len(fields) != 3 {
		writeError(w, http.StatusBadRequest, "invalid_json", "Request body must include starts_at_local, listing_version, and idempotency_key")
		return
	}
	for key := range fields {
		if key != "starts_at_local" && key != "listing_version" && key != "idempotency_key" {
			writeError(w, http.StatusBadRequest, "invalid_json", "Request body includes unsupported appointment fields")
			return
		}
	}
	var input struct {
		StartsAtLocal  string
		ListingVersion int
		IdempotencyKey string
	}
	if json.Unmarshal(fields["starts_at_local"], &input.StartsAtLocal) != nil ||
		json.Unmarshal(fields["listing_version"], &input.ListingVersion) != nil ||
		json.Unmarshal(fields["idempotency_key"], &input.IdempotencyKey) != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Appointment fields have invalid types")
		return
	}
	booking, err := h.app.Bookings.Request(
		r.Context(), &bookingsvc.Actor{ID: user.ID, Role: user.Role},
		r.PathValue("slug"), input.StartsAtLocal, input.ListingVersion, input.IdempotencyKey,
	)
	if err != nil {
		writeServiceBookingError(w, err)
		return
	}
	writeData(w, http.StatusCreated, booking)
}

func (h *Handler) accountServiceBookings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	user := auth.GetUserFromContext(r.Context())
	if user == nil || user.Role != db.RoleUser {
		writeError(w, http.StatusForbidden, "forbidden", "A customer account is required")
		return
	}
	page, limit := pageParams(r)
	if page > 1000 || (page-1)*limit > 50000 {
		writeError(w, http.StatusBadRequest, "invalid_pagination", "Page must be between 1 and 1000")
		return
	}
	items, total, err := h.app.Bookings.ForCustomer(
		r.Context(), &bookingsvc.Actor{ID: user.ID, Role: user.Role}, page, limit,
	)
	if err != nil {
		writeServiceBookingError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"items": items, "page": page, "limit": limit, "total": total,
	})
}

func (h *Handler) cancelServiceBooking(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	user := auth.GetUserFromContext(r.Context())
	if user == nil || user.Role != db.RoleUser {
		writeError(w, http.StatusForbidden, "forbidden", "A customer account is required")
		return
	}
	id, ok := parsePositiveID(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_booking_id", "Appointment id must be a positive integer")
		return
	}
	version, err := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("version")), 10, 64)
	if err != nil || version <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_version", "Current appointment version is required")
		return
	}
	if err := h.app.Bookings.Cancel(
		r.Context(), &bookingsvc.Actor{ID: user.ID, Role: user.Role}, id, version,
	); err != nil {
		writeServiceBookingError(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

func writeServiceBookingError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, bookingsvc.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "A customer account is required")
	case errors.Is(err, bookingsvc.ErrNotFound), errors.Is(err, db.ErrServiceNotFound):
		writeError(w, http.StatusNotFound, "booking_not_found", "Appointment or service not found")
	case errors.Is(err, bookingsvc.ErrInvalid):
		writeError(w, http.StatusBadRequest, "invalid_booking", "Check the selected time, listing version, and idempotency key")
	case errors.Is(err, bookingsvc.ErrListingChanged):
		writeError(w, http.StatusConflict, "service_changed", "This service changed. Reload its latest details before requesting")
	case errors.Is(err, bookingsvc.ErrTimeConflict):
		writeError(w, http.StatusConflict, "booking_conflict", "That time has already been requested")
	case errors.Is(err, bookingsvc.ErrUnavailable):
		writeError(w, http.StatusConflict, "booking_unavailable", "The service is not available at that time")
	case errors.Is(err, bookingsvc.ErrIdempotencyConflict):
		writeError(w, http.StatusConflict, "idempotency_conflict", "This key was already used for another request")
	case errors.Is(err, bookingsvc.ErrVersionConflict), errors.Is(err, bookingsvc.ErrInvalidState):
		writeError(w, http.StatusConflict, "booking_changed", "This appointment changed or can no longer be cancelled")
	default:
		writeError(w, http.StatusInternalServerError, "booking_unavailable", "Unable to process the appointment request")
	}
}

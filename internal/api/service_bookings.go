package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/auth"
	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/domain"
	bookingsvc "github.com/kyambuthia/sokomoko/internal/service/booking"
)

func (h *Handler) serviceAvailability(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	reader, ok := h.app.Bookings.(interface {
		Availability(context.Context, string, string) (bookingsvc.Availability, error)
	})
	if !ok {
		writeError(w, http.StatusInternalServerError, "availability_unavailable", "Service availability is unavailable")
		return
	}
	availability, err := reader.Availability(r.Context(), r.PathValue("slug"), r.URL.Query().Get("date"))
	switch {
	case errors.Is(err, bookingsvc.ErrInvalid):
		writeError(w, http.StatusBadRequest, "invalid_date", "A valid service slug and date in YYYY-MM-DD format are required")
		return
	case errors.Is(err, db.ErrServiceNotFound):
		writeError(w, http.StatusNotFound, "service_not_found", "Service listing not found")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "availability_unavailable", "Unable to load service availability")
		return
	}
	slots := make([]map[string]any, 0, len(availability.Slots))
	for _, slot := range availability.Slots {
		slots = append(slots, map[string]any{
			"local_start": slot.LocalStart,
			"starts_at":   slot.StartsAt.UTC(),
			"ends_at":     slot.EndsAt.UTC(),
		})
	}
	writeData(w, http.StatusOK, map[string]any{
		"service_slug":     availability.ServiceSlug,
		"listing_version":  availability.ListingVersion,
		"date":             availability.Date,
		"timezone":         availability.Timezone,
		"duration_minutes": availability.DurationMinutes,
		"slots":            slots,
	})
}

func (h *Handler) createServiceBooking(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if h.app.DemoMode {
		writeError(w, http.StatusForbidden, "demo_mode", "This demo does not accept appointment requests")
		return
	}
	user := auth.GetUserFromContext(r.Context())
	if user == nil || user.Role != domain.RoleUser {
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
		methodNotAllowed(w, http.MethodGet)
		return
	}
	user := auth.GetUserFromContext(r.Context())
	if user == nil || user.Role != domain.RoleUser {
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

func (h *Handler) partnerServiceBookings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	user := auth.GetUserFromContext(r.Context())
	if user == nil || user.Role != domain.RolePartner {
		writeError(w, http.StatusForbidden, "forbidden", "A partner account is required")
		return
	}
	page, limit := pageParams(r)
	if page > 1000 || (page-1)*limit > 50000 {
		writeError(w, http.StatusBadRequest, "invalid_pagination", "Page must be between 1 and 1000")
		return
	}
	items, total, err := h.app.Bookings.ForPartner(
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

func (h *Handler) partnerServiceBookingAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	user := auth.GetUserFromContext(r.Context())
	if user == nil || user.Role != domain.RolePartner {
		writeError(w, http.StatusForbidden, "forbidden", "A partner account is required")
		return
	}
	id, ok := parsePositiveID(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_booking_id", "Appointment id must be a positive integer")
		return
	}
	var fields map[string]json.RawMessage
	if err := decodeJSON(r, &fields); err != nil || len(fields) != 2 {
		writeError(w, http.StatusBadRequest, "invalid_json", "Request body must include action and version")
		return
	}
	for key := range fields {
		if key != "action" && key != "version" {
			writeError(w, http.StatusBadRequest, "invalid_json", "Request body includes unsupported appointment fields")
			return
		}
	}
	var action string
	var version int64
	if json.Unmarshal(fields["action"], &action) != nil ||
		json.Unmarshal(fields["version"], &version) != nil || version <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_booking_action", "Action and positive version are required")
		return
	}
	if err := h.app.Bookings.Respond(
		r.Context(), &bookingsvc.Actor{ID: user.ID, Role: user.Role}, id, version, action,
	); err != nil {
		writeServiceBookingError(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

func (h *Handler) cancelServiceBooking(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	user := auth.GetUserFromContext(r.Context())
	if user == nil || user.Role != domain.RoleUser {
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
		writeError(w, http.StatusForbidden, "forbidden", "This account cannot manage that appointment")
	case errors.Is(err, bookingsvc.ErrNotFound), errors.Is(err, db.ErrServiceNotFound):
		writeError(w, http.StatusNotFound, "booking_not_found", "Appointment or service not found")
	case errors.Is(err, bookingsvc.ErrInvalid):
		writeError(w, http.StatusBadRequest, "invalid_booking", "Check the appointment details and action")
	case errors.Is(err, bookingsvc.ErrListingChanged):
		writeError(w, http.StatusConflict, "service_changed", "This service changed. Reload its latest details before requesting")
	case errors.Is(err, bookingsvc.ErrTimeConflict):
		writeError(w, http.StatusConflict, "booking_conflict", "That time has already been requested")
	case errors.Is(err, bookingsvc.ErrUnavailable):
		writeError(w, http.StatusConflict, "booking_unavailable", "The service is not available at that time")
	case errors.Is(err, bookingsvc.ErrIdempotencyConflict):
		writeError(w, http.StatusConflict, "idempotency_conflict", "This key was already used for another request")
	case errors.Is(err, bookingsvc.ErrVersionConflict), errors.Is(err, bookingsvc.ErrInvalidState):
		writeError(w, http.StatusConflict, "booking_changed", "This appointment changed or cannot be updated with that action")
	case errors.Is(err, bookingsvc.ErrResponseExpired):
		writeError(w, http.StatusConflict, "response_expired", "The response deadline passed; this request is no longer active")
	default:
		writeError(w, http.StatusInternalServerError, "booking_unavailable", "Unable to process the appointment request")
	}
}

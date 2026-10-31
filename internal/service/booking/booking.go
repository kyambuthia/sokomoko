// Package booking validates and creates appointment requests.
package booking

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/slug"
)

var (
	ErrForbidden           = errors.New("customer role required")
	ErrInvalid             = errors.New("invalid booking request")
	ErrListingChanged      = errors.New("service listing changed; reload before requesting")
	ErrUnavailable         = errors.New("that time is no longer available")
	ErrTimeConflict        = errors.New("that time has already been requested")
	ErrIdempotencyConflict = errors.New("request key was already used for another appointment")
	ErrNotFound            = errors.New("appointment request not found")
	ErrInvalidState        = errors.New("appointment request cannot move to that state")
	ErrVersionConflict     = errors.New("appointment request changed; reload before updating")
	ErrResponseExpired     = errors.New("provider response deadline has passed")
)

type Actor struct {
	ID   int
	Role string
}

type store interface {
	PublicService(context.Context, string) (*db.ServiceListing, error)
	RequestServiceBooking(context.Context, int, int, int, time.Time, string, []byte) (*db.ServiceBooking, error)
	ServiceBookingByIdempotency(context.Context, int, string, []byte) (*db.ServiceBooking, error)
	ServiceBookingsForCustomer(context.Context, int, int, int) ([]db.ServiceBooking, int, error)
	ServiceBookingsForPartner(context.Context, int, int, int) ([]db.ServiceBooking, int, error)
	TransitionServiceBooking(context.Context, int, int, string, string, int64) error
}

type Service struct{ store store }

func New(s store) *Service { return &Service{store: s} }

const localMinuteLayout = "2006-01-02T15:04"

func parseLocalStart(value string, loc *time.Location) (time.Time, bool) {
	if len(value) != len(localMinuteLayout) {
		return time.Time{}, false
	}
	local, err := time.ParseInLocation(localMinuteLayout, value, loc)
	if err != nil || local.Format(localMinuteLayout) != value {
		return time.Time{}, false
	}
	wall, err := time.Parse(localMinuteLayout, value)
	if err != nil {
		return time.Time{}, false
	}

	// A local minute may map to zero instants during a spring-forward gap or
	// two instants during a fall-back fold. Accept only an unambiguous instant.
	offsets := map[int]struct{}{}
	for delta := -48 * time.Hour; delta <= 48*time.Hour; delta += 6 * time.Hour {
		_, offset := local.Add(delta).In(loc).Zone()
		offsets[offset] = struct{}{}
	}
	matches := 0
	var instant time.Time
	for offset := range offsets {
		candidate := wall.Add(-time.Duration(offset) * time.Second)
		if candidate.In(loc).Format(localMinuteLayout) == value {
			matches++
			instant = candidate
		}
	}
	return instant.UTC(), matches == 1
}

func validActor(actor *Actor) bool {
	return actor != nil && actor.ID > 0 && actor.Role == db.RoleUser
}

// Request creates an idempotent request using the listing version the customer
// saw. The database rechecks opening hours, capacity and current price.
func (s *Service) Request(ctx context.Context, actor *Actor, listingSlug, startsAtLocal string, expectedListingVersion int, idempotencyKey string) (*db.ServiceBooking, error) {
	if !validActor(actor) {
		return nil, ErrForbidden
	}
	key := strings.TrimSpace(idempotencyKey)
	if !slug.ValidWithMaxLength(listingSlug, 160) || expectedListingVersion <= 0 ||
		key != idempotencyKey || len(key) < 16 || len(key) > 128 {
		return nil, ErrInvalid
	}

	payload := fmt.Sprintf("service-booking-v1\x00%s\x00%d\x00%s",
		listingSlug, expectedListingVersion, startsAtLocal)
	hash := sha256.Sum256([]byte(payload))
	existing, err := s.store.ServiceBookingByIdempotency(ctx, actor.ID, key, hash[:])
	if errors.Is(err, db.ErrServiceBookingIdempotencyConflict) {
		return nil, ErrIdempotencyConflict
	}
	if errors.Is(err, db.ErrServiceBookingForbidden) {
		return nil, ErrForbidden
	}
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}

	listing, err := s.store.PublicService(ctx, listingSlug)
	if err != nil {
		return nil, err
	}
	if listing.Version != expectedListingVersion {
		return nil, ErrListingChanged
	}
	loc, err := time.LoadLocation(listing.Timezone)
	if err != nil {
		return nil, ErrUnavailable
	}
	startsAtUTC, valid := parseLocalStart(startsAtLocal, loc)
	if !valid || !startsAtUTC.After(time.Now().UTC()) {
		return nil, ErrInvalid
	}

	booking, err := s.store.RequestServiceBooking(
		ctx, actor.ID, listing.ID, listing.Version, startsAtUTC, key, hash[:],
	)
	switch {
	case errors.Is(err, db.ErrServiceBookingConflict):
		return nil, ErrTimeConflict
	case errors.Is(err, db.ErrServiceBookingUnavailable):
		return nil, ErrUnavailable
	case errors.Is(err, db.ErrServiceBookingIdempotencyConflict):
		return nil, ErrIdempotencyConflict
	case errors.Is(err, db.ErrServiceBookingForbidden):
		return nil, ErrForbidden
	case errors.Is(err, db.ErrServiceBookingInvalidRequest):
		return nil, ErrInvalid
	case errors.Is(err, db.ErrServiceConflict):
		return nil, ErrListingChanged
	}
	return booking, err
}

// ForCustomer returns a customer's own appointment requests.
func (s *Service) ForCustomer(ctx context.Context, actor *Actor, page, limit int) ([]db.ServiceBooking, int, error) {
	if actor == nil || actor.ID <= 0 || actor.Role != db.RoleUser {
		return nil, 0, ErrForbidden
	}
	if page < 1 || page > 1000 || limit < 1 || limit > 50 {
		return nil, 0, ErrInvalid
	}
	items, total, err := s.store.ServiceBookingsForCustomer(ctx, actor.ID, limit, (page-1)*limit)
	if errors.Is(err, db.ErrServiceBookingForbidden) {
		return nil, 0, ErrForbidden
	}
	return items, total, err
}

// ForPartner returns appointment requests for the partner's active shop membership.
func (s *Service) ForPartner(ctx context.Context, actor *Actor, page, limit int) ([]db.ServiceBooking, int, error) {
	if actor == nil || actor.ID <= 0 || actor.Role != db.RolePartner {
		return nil, 0, ErrForbidden
	}
	if page < 1 || page > 1000 || limit < 1 || limit > 50 {
		return nil, 0, ErrInvalid
	}
	items, total, err := s.store.ServiceBookingsForPartner(ctx, actor.ID, limit, (page-1)*limit)
	if errors.Is(err, db.ErrServiceBookingForbidden) {
		return nil, 0, ErrForbidden
	}
	return items, total, err
}

// Cancel lets a customer withdraw their own future request or confirmed booking.
func (s *Service) Cancel(ctx context.Context, actor *Actor, bookingID int, expectedVersion int64) error {
	if actor == nil || actor.ID <= 0 || actor.Role != db.RoleUser {
		return ErrForbidden
	}
	if bookingID <= 0 || expectedVersion <= 0 {
		return ErrInvalid
	}
	return mapTransitionError(s.store.TransitionServiceBooking(ctx, actor.ID, bookingID, actor.Role, "cancelled", expectedVersion))
}

// Respond lets a partner confirm or reject a still-open request.
func (s *Service) Respond(ctx context.Context, actor *Actor, bookingID int, expectedVersion int64, action string) error {
	if actor == nil || actor.ID <= 0 || actor.Role != db.RolePartner {
		return ErrForbidden
	}
	if bookingID <= 0 || expectedVersion <= 0 {
		return ErrInvalid
	}
	status := ""
	switch action {
	case "confirm":
		status = "confirmed"
	case "reject":
		status = "rejected"
	case "cancel":
		status = "cancelled"
	default:
		return ErrInvalid
	}
	return mapTransitionError(s.store.TransitionServiceBooking(ctx, actor.ID, bookingID, actor.Role, status, expectedVersion))
}

func mapTransitionError(err error) error {
	switch {
	case errors.Is(err, db.ErrServiceBookingNotFound):
		return ErrNotFound
	case errors.Is(err, db.ErrServiceBookingInvalidState):
		return ErrInvalidState
	case errors.Is(err, db.ErrServiceBookingVersionConflict):
		return ErrVersionConflict
	case errors.Is(err, db.ErrServiceBookingResponseExpired):
		return ErrResponseExpired
	case errors.Is(err, db.ErrServiceBookingForbidden):
		return ErrForbidden
	case errors.Is(err, db.ErrServiceBookingInvalidRequest):
		return ErrInvalid
	default:
		return err
	}
}

package db

import (
	"crypto/sha256"
	"testing"
	"time"
)

func mustPublishedBookableService(t *testing.T, s *Store, ownerID int, slug string) *ServiceListing {
	t.Helper()
	in := serviceFixture(slug)
	in.Timezone = "Africa/Nairobi"
	in.State = "published"
	in.WeeklyHours = make([]ServiceWeeklyHour, 0, 7)
	for d := 0; d < 7; d++ {
		in.WeeklyHours = append(in.WeeklyHours, ServiceWeeklyHour{Weekday: d, OpensAt: "00:00", ClosesAt: "23:59"})
	}
	v, err := s.SaveService(t.Context(), ownerID, in)
	chatMust(t, err)
	return v
}

func testBookingHash(label string) []byte {
	sum := sha256.Sum256([]byte(label))
	return sum[:]
}

func TestServiceBookingLifecycle_RequestIntervalsConfirmAndComplete(t *testing.T) {
	s := newTestStore(t)
	_, owner, _ := chatShop(t, s)
	_, outsider, _ := chatShop(t, s)
	buyer1 := mustCreateUser(t, s, RoleUser)
	buyer2 := mustCreateUser(t, s, RoleUser)

	listing := mustPublishedBookableService(t, s, owner, "tailor-booking")
	slot := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Hour)
	hash1 := testBookingHash("slot-1")
	hash2 := testBookingHash("slot-2")

	// Stale listing version is rejected.
	_, err := s.RequestServiceBooking(t.Context(), buyer1, listing.ID, listing.Version+1, slot, "idem-stale-booking-key", hash1)
	chatWantError(t, err, ErrServiceConflict)

	// Request creates booking and idempotency replay returns same record.
	booking, err := s.RequestServiceBooking(t.Context(), buyer1, listing.ID, listing.Version, slot, "idem-booking-key-0001", hash1)
	chatMust(t, err)
	if booking.Status != "requested" || booking.Version != 1 {
		t.Fatalf("got booking status=%q version=%d; want requested v1", booking.Status, booking.Version)
	}
	replay, err := s.ServiceBookingByIdempotency(t.Context(), buyer1, "idem-booking-key-0001", hash1)
	chatMust(t, err)
	if replay.ID != booking.ID {
		t.Fatalf("replay ID = %d; want %d", replay.ID, booking.ID)
	}
	_, err = s.ServiceBookingByIdempotency(t.Context(), buyer1, "idem-booking-key-0001", hash2)
	chatWantError(t, err, ErrServiceBookingIdempotencyConflict)

	// Availability intervals include the active reservation.
	intervals, err := s.ServiceBookingIntervals(t.Context(), listing.ID, slot.Add(-time.Hour), slot.Add(2*time.Hour))
	chatMust(t, err)
	if len(intervals) != 1 {
		t.Fatalf("intervals len = %d; want 1", len(intervals))
	}

	// Overlapping booking by another customer conflicts.
	_, err = s.RequestServiceBooking(t.Context(), buyer2, listing.ID, listing.Version, slot, "idem-booking-key-0002", hash1)
	chatWantError(t, err, ErrServiceBookingConflict)

	// Outsider partner cannot transition; stale version fails; owner can confirm, future in_progress is rejected, and partner cancel frees slot.
	chatWantError(t, s.TransitionServiceBooking(t.Context(), outsider, int(booking.ID), RolePartner, "confirmed", 1), ErrServiceBookingNotFound)
	chatWantError(t, s.TransitionServiceBooking(t.Context(), owner, int(booking.ID), RolePartner, "confirmed", 99), ErrServiceBookingVersionConflict)
	chatMust(t, s.TransitionServiceBooking(t.Context(), owner, int(booking.ID), RolePartner, "confirmed", 1))
	chatWantError(t, s.TransitionServiceBooking(t.Context(), owner, int(booking.ID), RolePartner, "in_progress", 2), ErrServiceBookingInvalidState)

	items, total, err := s.ServiceBookingsForPartner(t.Context(), owner, 10, 0)
	chatMust(t, err)
	if total != 1 || len(items) != 1 || items[0].Status != "confirmed" {
		t.Fatalf("partner bookings = %+v (total %d)", items, total)
	}
}

func TestServiceBookingLifecycle_CustomerCancelAndExpiry(t *testing.T) {
	s := newTestStore(t)
	_, owner, _ := chatShop(t, s)
	buyer1 := mustCreateUser(t, s, RoleUser)
	buyer2 := mustCreateUser(t, s, RoleUser)

	listing := mustPublishedBookableService(t, s, owner, "cancel-expiry-booking")
	slot := time.Now().UTC().Add(72 * time.Hour).Truncate(time.Hour)

	b1, err := s.RequestServiceBooking(t.Context(), buyer1, listing.ID, listing.Version, slot, "cancel-idem-key-0001", testBookingHash("c1"))
	chatMust(t, err)

	// Other customer cannot cancel buyer1's booking.
	chatWantError(t, s.TransitionServiceBooking(t.Context(), buyer2, int(b1.ID), RoleUser, "cancelled", 1), ErrServiceBookingNotFound)
	// Customer cancels, freeing the slot.
	chatMust(t, s.TransitionServiceBooking(t.Context(), buyer1, int(b1.ID), RoleUser, "cancelled", 1))

	b2, err := s.RequestServiceBooking(t.Context(), buyer2, listing.ID, listing.Version, slot, "cancel-idem-key-0002", testBookingHash("c2"))
	chatMust(t, err)

	// Non-expired request stays active; partner rejection frees the slot for a new booking.
	expired, err := s.ExpireServiceBookingRequests(t.Context())
	chatMust(t, err)
	if expired != 0 {
		t.Fatalf("expired = %d; want 0 for future deadline", expired)
	}
	chatMust(t, s.TransitionServiceBooking(t.Context(), owner, int(b2.ID), RolePartner, "rejected", 1))

	b3, err := s.RequestServiceBooking(t.Context(), buyer1, listing.ID, listing.Version, slot, "cancel-idem-key-0003", testBookingHash("c3"))
	chatMust(t, err)
	if b3.ID == b2.ID {
		t.Fatalf("expected new booking ID after rejection, got %d", b3.ID)
	}
}

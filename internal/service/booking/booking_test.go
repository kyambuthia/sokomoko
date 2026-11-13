package booking

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/domain"
)

type mockBookingStore struct {
	publicServiceFn               func(context.Context, string) (*db.ServiceListing, error)
	requestServiceBookingFn       func(context.Context, int, int, int, time.Time, string, []byte) (*db.ServiceBooking, error)
	serviceBookingByIdempotencyFn func(context.Context, int, string, []byte) (*db.ServiceBooking, error)
	serviceBookingsForCustomerFn  func(context.Context, int, int, int) ([]db.ServiceBooking, int, error)
	serviceBookingsForPartnerFn   func(context.Context, int, int, int) ([]db.ServiceBooking, int, error)
	transitionServiceBookingFn    func(context.Context, int, int, string, string, int64) error
	serviceBookingIntervalsFn     func(context.Context, int, time.Time, time.Time) ([]db.ServiceBookingInterval, error)
}

func (m *mockBookingStore) PublicService(ctx context.Context, slug string) (*db.ServiceListing, error) {
	if m.publicServiceFn != nil {
		return m.publicServiceFn(ctx, slug)
	}
	return nil, nil
}

func (m *mockBookingStore) RequestServiceBooking(ctx context.Context, customerID, serviceID, version int, startsAt time.Time, key string, hash []byte) (*db.ServiceBooking, error) {
	if m.requestServiceBookingFn != nil {
		return m.requestServiceBookingFn(ctx, customerID, serviceID, version, startsAt, key, hash)
	}
	return nil, nil
}

func (m *mockBookingStore) ServiceBookingByIdempotency(ctx context.Context, customerID int, key string, hash []byte) (*db.ServiceBooking, error) {
	if m.serviceBookingByIdempotencyFn != nil {
		return m.serviceBookingByIdempotencyFn(ctx, customerID, key, hash)
	}
	return nil, nil
}

func (m *mockBookingStore) ServiceBookingsForCustomer(ctx context.Context, customerID, limit, offset int) ([]db.ServiceBooking, int, error) {
	if m.serviceBookingsForCustomerFn != nil {
		return m.serviceBookingsForCustomerFn(ctx, customerID, limit, offset)
	}
	return nil, 0, nil
}

func (m *mockBookingStore) ServiceBookingsForPartner(ctx context.Context, partnerUserID, limit, offset int) ([]db.ServiceBooking, int, error) {
	if m.serviceBookingsForPartnerFn != nil {
		return m.serviceBookingsForPartnerFn(ctx, partnerUserID, limit, offset)
	}
	return nil, 0, nil
}

func (m *mockBookingStore) TransitionServiceBooking(ctx context.Context, actorID, bookingID int, role, status string, version int64) error {
	if m.transitionServiceBookingFn != nil {
		return m.transitionServiceBookingFn(ctx, actorID, bookingID, role, status, version)
	}
	return nil
}

func (m *mockBookingStore) ServiceBookingIntervals(ctx context.Context, serviceID int, from, to time.Time) ([]db.ServiceBookingInterval, error) {
	if m.serviceBookingIntervalsFn != nil {
		return m.serviceBookingIntervalsFn(ctx, serviceID, from, to)
	}
	return nil, nil
}

func TestRequestValidation(t *testing.T) {
	ctx := context.Background()

	t.Run("non-user actor forbidden", func(t *testing.T) {
		svc := New(&mockBookingStore{})

		actors := []*Actor{
			nil,
			{ID: 0, Role: domain.RoleUser},
			{ID: 1, Role: domain.RolePartner},
			{ID: 2, Role: domain.RoleAdmin},
		}
		for _, a := range actors {
			_, err := svc.Request(ctx, a, "haircut", "2026-12-01T10:00", 1, "valid-idempotency-key-12345")
			if !errors.Is(err, ErrForbidden) {
				t.Fatalf("expected ErrForbidden for actor %+v, got %v", a, err)
			}
		}
	})

	t.Run("invalid parameters return ErrInvalid without db calls", func(t *testing.T) {
		svc := New(&mockBookingStore{
			serviceBookingByIdempotencyFn: func(ctx context.Context, customerID int, key string, hash []byte) (*db.ServiceBooking, error) {
				t.Fatal("store must not be called")
				return nil, nil
			},
		})
		actor := &Actor{ID: 1, Role: domain.RoleUser}

		// Invalid slug
		if _, err := svc.Request(ctx, actor, "invalid slug with spaces", "2026-12-01T10:00", 1, "valid-idempotency-key-12345"); !errors.Is(err, ErrInvalid) {
			t.Fatalf("expected ErrInvalid for invalid slug, got %v", err)
		}

		// Invalid listing version
		if _, err := svc.Request(ctx, actor, "haircut", "2026-12-01T10:00", 0, "valid-idempotency-key-12345"); !errors.Is(err, ErrInvalid) {
			t.Fatalf("expected ErrInvalid for version <= 0, got %v", err)
		}

		// Untrimmed or too short idempotency key
		if _, err := svc.Request(ctx, actor, "haircut", "2026-12-01T10:00", 1, "short"); !errors.Is(err, ErrInvalid) {
			t.Fatalf("expected ErrInvalid for short idempotency key, got %v", err)
		}
		if _, err := svc.Request(ctx, actor, "haircut", "2026-12-01T10:00", 1, "  valid-idempotency-key-12345  "); !errors.Is(err, ErrInvalid) {
			t.Fatalf("expected ErrInvalid for untrimmed idempotency key, got %v", err)
		}
	})

	t.Run("idempotency returns existing request", func(t *testing.T) {
		actor := &Actor{ID: 10, Role: domain.RoleUser}
		existingBooking := &db.ServiceBooking{
			ID:               42,
			CustomerID:       10,
			ServiceListingID: 5,
			Status:           "requested",
		}
		svc := New(&mockBookingStore{
			serviceBookingByIdempotencyFn: func(ctx context.Context, customerID int, key string, hash []byte) (*db.ServiceBooking, error) {
				return existingBooking, nil
			},
		})

		got, err := svc.Request(ctx, actor, "haircut", "2026-12-01T10:00", 1, "valid-idempotency-key-12345")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != existingBooking {
			t.Fatalf("expected %v, got %v", existingBooking, got)
		}
	})

	t.Run("listing changed returns ErrListingChanged", func(t *testing.T) {
		actor := &Actor{ID: 10, Role: domain.RoleUser}
		svc := New(&mockBookingStore{
			serviceBookingByIdempotencyFn: func(ctx context.Context, customerID int, key string, hash []byte) (*db.ServiceBooking, error) {
				return nil, nil
			},
			publicServiceFn: func(ctx context.Context, slug string) (*db.ServiceListing, error) {
				return &db.ServiceListing{
					ID:              5,
					Slug:            slug,
					Version:         2, // customer expected version 1
					Timezone:        "UTC",
					DurationMinutes: 30,
				}, nil
			},
		})

		_, err := svc.Request(ctx, actor, "haircut", "2026-12-01T10:00", 1, "valid-idempotency-key-12345")
		if !errors.Is(err, ErrListingChanged) {
			t.Fatalf("expected ErrListingChanged, got %v", err)
		}
	})

	t.Run("maps store error codes", func(t *testing.T) {
		actor := &Actor{ID: 10, Role: domain.RoleUser}
		futureTime := time.Now().Add(48 * time.Hour).UTC().Format("2006-01-02T15:04")

		tests := []struct {
			storeErr error
			wantErr  error
		}{
			{db.ErrServiceBookingConflict, ErrTimeConflict},
			{db.ErrServiceBookingUnavailable, ErrUnavailable},
			{db.ErrServiceBookingIdempotencyConflict, ErrIdempotencyConflict},
			{db.ErrServiceBookingForbidden, ErrForbidden},
			{db.ErrServiceBookingInvalidRequest, ErrInvalid},
			{db.ErrServiceConflict, ErrListingChanged},
		}

		for _, tc := range tests {
			svc := New(&mockBookingStore{
				serviceBookingByIdempotencyFn: func(ctx context.Context, customerID int, key string, hash []byte) (*db.ServiceBooking, error) {
					return nil, nil
				},
				publicServiceFn: func(ctx context.Context, slug string) (*db.ServiceListing, error) {
					return &db.ServiceListing{
						ID:              5,
						Slug:            slug,
						Version:         1,
						Timezone:        "UTC",
						DurationMinutes: 30,
					}, nil
				},
				requestServiceBookingFn: func(ctx context.Context, customerID, serviceID, version int, startsAt time.Time, key string, hash []byte) (*db.ServiceBooking, error) {
					return nil, tc.storeErr
				},
			})

			_, err := svc.Request(ctx, actor, "haircut", futureTime, 1, "valid-idempotency-key-12345")
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected error %v for store error %v, got %v", tc.wantErr, tc.storeErr, err)
			}
		}
	})
}

func TestForCustomerAndForPartner(t *testing.T) {
	ctx := context.Background()

	t.Run("ForCustomer role check and paging", func(t *testing.T) {
		svc := New(&mockBookingStore{})

		if _, _, err := svc.ForCustomer(ctx, &Actor{ID: 1, Role: domain.RolePartner}, 1, 10); !errors.Is(err, ErrForbidden) {
			t.Fatalf("expected ErrForbidden for partner actor")
		}
		if _, _, err := svc.ForCustomer(ctx, &Actor{ID: 1, Role: domain.RoleUser}, 0, 10); !errors.Is(err, ErrInvalid) {
			t.Fatalf("expected ErrInvalid for page < 1")
		}
		if _, _, err := svc.ForCustomer(ctx, &Actor{ID: 1, Role: domain.RoleUser}, 1, 100); !errors.Is(err, ErrInvalid) {
			t.Fatalf("expected ErrInvalid for limit > 50")
		}
	})

	t.Run("ForPartner role check and paging", func(t *testing.T) {
		svc := New(&mockBookingStore{})

		if _, _, err := svc.ForPartner(ctx, &Actor{ID: 1, Role: domain.RoleUser}, 1, 10); !errors.Is(err, ErrForbidden) {
			t.Fatalf("expected ErrForbidden for customer actor")
		}
		if _, _, err := svc.ForPartner(ctx, &Actor{ID: 1, Role: domain.RolePartner}, 0, 10); !errors.Is(err, ErrInvalid) {
			t.Fatalf("expected ErrInvalid for page < 1")
		}
		if _, _, err := svc.ForPartner(ctx, &Actor{ID: 1, Role: domain.RolePartner}, 1, 100); !errors.Is(err, ErrInvalid) {
			t.Fatalf("expected ErrInvalid for limit > 50")
		}
	})
}

func TestCancelAndRespondTransitions(t *testing.T) {
	ctx := context.Background()

	t.Run("Cancel role check and mapping", func(t *testing.T) {
		svc := New(&mockBookingStore{
			transitionServiceBookingFn: func(ctx context.Context, actorID, bookingID int, role, status string, version int64) error {
				if status != "cancelled" {
					t.Fatalf("expected status cancelled, got %s", status)
				}
				return db.ErrServiceBookingVersionConflict
			},
		})

		// Non-user
		if err := svc.Cancel(ctx, &Actor{ID: 1, Role: domain.RolePartner}, 10, 1); !errors.Is(err, ErrForbidden) {
			t.Fatalf("expected ErrForbidden")
		}
		// Invalid params
		if err := svc.Cancel(ctx, &Actor{ID: 1, Role: domain.RoleUser}, 0, 1); !errors.Is(err, ErrInvalid) {
			t.Fatalf("expected ErrInvalid")
		}
		// Version conflict mapped
		if err := svc.Cancel(ctx, &Actor{ID: 1, Role: domain.RoleUser}, 10, 1); !errors.Is(err, ErrVersionConflict) {
			t.Fatalf("expected ErrVersionConflict, got %v", err)
		}
	})

	t.Run("Respond actions and role check", func(t *testing.T) {
		var lastStatus string
		svc := New(&mockBookingStore{
			transitionServiceBookingFn: func(ctx context.Context, actorID, bookingID int, role, status string, version int64) error {
				lastStatus = status
				return nil
			},
		})
		actor := &Actor{ID: 5, Role: domain.RolePartner}

		// Non-partner
		if err := svc.Respond(ctx, &Actor{ID: 5, Role: domain.RoleUser}, 10, 1, "confirm"); !errors.Is(err, ErrForbidden) {
			t.Fatalf("expected ErrForbidden")
		}

		// Invalid action
		if err := svc.Respond(ctx, actor, 10, 1, "fly_to_moon"); !errors.Is(err, ErrInvalid) {
			t.Fatalf("expected ErrInvalid")
		}

		actions := map[string]string{
			"confirm":  "confirmed",
			"reject":   "rejected",
			"cancel":   "cancelled",
			"start":    "in_progress",
			"complete": "completed",
			"no_show":  "no_show",
		}
		for action, expectedStatus := range actions {
			if err := svc.Respond(ctx, actor, 10, 1, action); err != nil {
				t.Fatalf("unexpected error for action %s: %v", action, err)
			}
			if lastStatus != expectedStatus {
				t.Fatalf("for action %s expected status %s, got %s", action, expectedStatus, lastStatus)
			}
		}
	})
}

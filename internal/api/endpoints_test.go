package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kyambuthia/sokomoko/internal/app"
	"github.com/kyambuthia/sokomoko/internal/auth"
	"github.com/kyambuthia/sokomoko/internal/domain"
	accountsvc "github.com/kyambuthia/sokomoko/internal/service/account"
	bookingsvc "github.com/kyambuthia/sokomoko/internal/service/booking"
	directorysvc "github.com/kyambuthia/sokomoko/internal/service/directory"
	logisticsvc "github.com/kyambuthia/sokomoko/internal/service/logistics"
	paymentsvc "github.com/kyambuthia/sokomoko/internal/service/payment"
)

type stubAuthService struct {
	app.AuthService
	user *auth.User
}

func (s *stubAuthService) APIAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.user != nil {
			r = r.WithContext(auth.WithUser(r.Context(), s.user))
		}
		next.ServeHTTP(w, r)
	})
}

type stubDirectoryService struct {
	app.DirectoryService
	browseFn func(context.Context, directorysvc.ServiceFilter, int) ([]directorysvc.ServiceListing, int, error)
	publicFn func(context.Context, string) (*directorysvc.ServiceListing, error)
}

func (s *stubDirectoryService) Browse(ctx context.Context, filter directorysvc.ServiceFilter, page int) ([]directorysvc.ServiceListing, int, error) {
	return s.browseFn(ctx, filter, page)
}

func (s *stubDirectoryService) Public(ctx context.Context, slug string) (*directorysvc.ServiceListing, error) {
	return s.publicFn(ctx, slug)
}

type stubBookingService struct {
	app.BookingService
	availabilityFn func(context.Context, string, string) (bookingsvc.Availability, error)
}

func (s *stubBookingService) Availability(ctx context.Context, slug, date string) (bookingsvc.Availability, error) {
	return s.availabilityFn(ctx, slug, date)
}

type stubLogisticsService struct {
	app.LogisticsService
	jobsByViewFn func(context.Context, *logisticsvc.Actor, string) ([]logisticsvc.CourierDelivery, error)
}

func (s *stubLogisticsService) JobsByView(ctx context.Context, actor *logisticsvc.Actor, view string) ([]logisticsvc.CourierDelivery, error) {
	return s.jobsByViewFn(ctx, actor, view)
}

type stubAccountService struct {
	app.AccountService
	orderForUserFn func(context.Context, int, int) (*accountsvc.Order, error)
}

func (s *stubAccountService) OrderForUser(ctx context.Context, userID, orderID int) (*accountsvc.Order, error) {
	return s.orderForUserFn(ctx, userID, orderID)
}

type stubPaymentService struct {
	app.PaymentService
	statusFn func(context.Context, int, int) (paymentsvc.StatusView, error)
}

func (s *stubPaymentService) Status(ctx context.Context, userID, orderID int) (paymentsvc.StatusView, error) {
	return s.statusFn(ctx, userID, orderID)
}

func TestServicesEndpoints(t *testing.T) {
	t.Run("services validates method, partner_id, and maps errors", func(t *testing.T) {
		var gotFilter directorysvc.ServiceFilter
		var gotPage int
		h := &Handler{app: app.New(app.Dependencies{
			Directory: &stubDirectoryService{
				browseFn: func(_ context.Context, f directorysvc.ServiceFilter, page int) ([]directorysvc.ServiceListing, int, error) {
					gotFilter, gotPage = f, page
					if f.Query == "boom" {
						return nil, 0, errors.New("db error")
					}
					return []directorysvc.ServiceListing{{ID: 1, Slug: "tailor", Title: "Tailoring"}}, 1, nil
				},
			},
		})}

		postRec := httptest.NewRecorder()
		h.serviceList(postRec, httptest.NewRequest(http.MethodPost, apiPrefix+"/services", nil))
		if postRec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("POST status = %d; want 405", postRec.Code)
		}

		badPartnerRec := httptest.NewRecorder()
		h.serviceList(badPartnerRec, httptest.NewRequest(http.MethodGet, apiPrefix+"/services?partner_id=abc", nil))
		if badPartnerRec.Code != http.StatusBadRequest {
			t.Fatalf("bad partner_id status = %d; want 400", badPartnerRec.Code)
		}
		if errBody := decodeErrEnvelope(t, badPartnerRec); errBody.Code != "invalid_service" {
			t.Fatalf("error code = %q; want invalid_service", errBody.Code)
		}

		errRec := httptest.NewRecorder()
		h.serviceList(errRec, httptest.NewRequest(http.MethodGet, apiPrefix+"/services?q=boom", nil))
		if errRec.Code != http.StatusInternalServerError {
			t.Fatalf("error status = %d; want 500", errRec.Code)
		}

		okRec := httptest.NewRecorder()
		h.serviceList(okRec, httptest.NewRequest(http.MethodGet, apiPrefix+"/services?page=0&limit=999&partner_id=5", nil))
		if okRec.Code != http.StatusOK {
			t.Fatalf("ok status = %d; want 200", okRec.Code)
		}
		if gotPage != 1 || gotFilter.Limit != maxPageLimit || gotFilter.PartnerID != 5 {
			t.Fatalf("got page=%d limit=%d partnerID=%d", gotPage, gotFilter.Limit, gotFilter.PartnerID)
		}
	})

	t.Run("serviceDetail maps missing and found services", func(t *testing.T) {
		h := &Handler{app: app.New(app.Dependencies{
			Directory: &stubDirectoryService{
				publicFn: func(_ context.Context, slug string) (*directorysvc.ServiceListing, error) {
					if slug == "missing" {
						return nil, domain.ErrServiceNotFound
					}
					return &directorysvc.ServiceListing{ID: 3, Slug: slug, Title: "Alterations"}, nil
				},
			},
		})}

		missRec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, apiPrefix+"/services/missing", nil)
		req.SetPathValue("slug", "missing")
		h.serviceDetail(missRec, req)
		if missRec.Code != http.StatusNotFound {
			t.Fatalf("missing status = %d; want 404", missRec.Code)
		}

		okRec := httptest.NewRecorder()
		okReq := httptest.NewRequest(http.MethodGet, apiPrefix+"/services/alterations", nil)
		okReq.SetPathValue("slug", "alterations")
		h.serviceDetail(okRec, okReq)
		if okRec.Code != http.StatusOK {
			t.Fatalf("ok status = %d; want 200", okRec.Code)
		}
	})

	t.Run("serviceAvailability maps invalid date, not found, and slots", func(t *testing.T) {
		h := &Handler{app: app.New(app.Dependencies{
			Bookings: &stubBookingService{
				availabilityFn: func(_ context.Context, slug, date string) (bookingsvc.Availability, error) {
					switch slug {
					case "bad-date":
						return bookingsvc.Availability{}, bookingsvc.ErrInvalid
					case "missing":
						return bookingsvc.Availability{}, domain.ErrServiceNotFound
					default:
						return bookingsvc.Availability{
							Timezone: "Africa/Nairobi",
							Date:     date,
							Slots: []bookingsvc.AvailabilitySlot{{
								LocalStart: "09:00",
								StartsAt:   time.Date(2026, 11, 13, 6, 0, 0, 0, time.UTC),
							}},
						}, nil
					}
				},
			},
		})}

		for _, tc := range []struct {
			slug string
			want int
		}{
			{"bad-date", http.StatusBadRequest},
			{"missing", http.StatusNotFound},
			{"tailor", http.StatusOK},
		} {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, apiPrefix+"/services/"+tc.slug+"/availability?date=2026-11-13", nil)
			req.SetPathValue("slug", tc.slug)
			h.serviceAvailability(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("slug=%s status = %d; want %d", tc.slug, rec.Code, tc.want)
			}
		}
	})
}

func TestRoleGuardAndLogisticsEndpoints(t *testing.T) {
	t.Run("role middleware enforces authentication and allowed roles", func(t *testing.T) {
		okHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		unauth := &Handler{app: app.New(app.Dependencies{Auth: &stubAuthService{}})}
		rec := httptest.NewRecorder()
		unauth.role([]string{domain.RoleCourier}, okHandler).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("unauth status = %d; want 401", rec.Code)
		}

		wrongRole := &Handler{app: app.New(app.Dependencies{Auth: &stubAuthService{user: &auth.User{ID: 1, Role: domain.RoleUser}}})}
		rec = httptest.NewRecorder()
		wrongRole.role([]string{domain.RoleCourier}, okHandler).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("wrong role status = %d; want 403", rec.Code)
		}

		allowed := &Handler{app: app.New(app.Dependencies{Auth: &stubAuthService{user: &auth.User{ID: 2, Role: domain.RoleCourier}}})}
		rec = httptest.NewRecorder()
		allowed.role([]string{domain.RoleCourier}, okHandler).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("allowed status = %d; want 200", rec.Code)
		}
	})

	t.Run("courierDeliveries maps invalid view and returns jobs", func(t *testing.T) {
		h := &Handler{app: app.New(app.Dependencies{
			Logistics: &stubLogisticsService{
				jobsByViewFn: func(_ context.Context, _ *logisticsvc.Actor, view string) ([]logisticsvc.CourierDelivery, error) {
					if view == "bad" {
						return nil, domain.ErrInvalidCourierView
					}
					return []logisticsvc.CourierDelivery{{ID: 9, OrderID: 12, Status: "assigned"}}, nil
				},
			},
		})}

		badRec := httptest.NewRecorder()
		badReq := httptest.NewRequest(http.MethodGet, apiPrefix+"/courier/deliveries?view=bad", nil)
		badReq = badReq.WithContext(auth.WithUser(badReq.Context(), &auth.User{ID: 2, Role: domain.RoleCourier}))
		h.courierDeliveries(badRec, badReq)
		if badRec.Code != http.StatusBadRequest {
			t.Fatalf("bad view status = %d; want 400", badRec.Code)
		}

		okRec := httptest.NewRecorder()
		okReq := httptest.NewRequest(http.MethodGet, apiPrefix+"/courier/deliveries", nil)
		okReq = okReq.WithContext(auth.WithUser(okReq.Context(), &auth.User{ID: 2, Role: domain.RoleCourier}))
		h.courierDeliveries(okRec, okReq)
		if okRec.Code != http.StatusOK {
			t.Fatalf("ok status = %d; want 200", okRec.Code)
		}
	})
}

func TestAccountAndCheckoutValidation(t *testing.T) {
	t.Run("accountOrder validates ID and maps not found", func(t *testing.T) {
		h := &Handler{app: app.New(app.Dependencies{
			Account: &stubAccountService{
				orderForUserFn: func(context.Context, int, int) (*accountsvc.Order, error) {
					return nil, nil
				},
			},
		})}

		badRec := httptest.NewRecorder()
		badReq := httptest.NewRequest(http.MethodGet, apiPrefix+"/account/orders/0", nil)
		badReq = badReq.WithContext(auth.WithUser(badReq.Context(), &auth.User{ID: 7, Role: domain.RoleUser}))
		h.accountOrder(badRec, badReq)
		if badRec.Code != http.StatusBadRequest {
			t.Fatalf("invalid order id status = %d; want 400", badRec.Code)
		}

		missRec := httptest.NewRecorder()
		missReq := httptest.NewRequest(http.MethodGet, apiPrefix+"/account/orders/42", nil)
		missReq = missReq.WithContext(auth.WithUser(missReq.Context(), &auth.User{ID: 7, Role: domain.RoleUser}))
		h.accountOrder(missRec, missReq)
		if missRec.Code != http.StatusNotFound {
			t.Fatalf("missing order status = %d; want 404", missRec.Code)
		}
	})

	t.Run("accountOrderPayment maps ErrPaymentNotFound", func(t *testing.T) {
		h := &Handler{app: app.New(app.Dependencies{
			Payment: &stubPaymentService{
				statusFn: func(context.Context, int, int) (paymentsvc.StatusView, error) {
					return paymentsvc.StatusView{}, domain.ErrPaymentNotFound
				},
			},
		})}

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, apiPrefix+"/account/orders/42/payment", nil)
		h.accountOrderPayment(rec, req, 7, "42")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d; want 404", rec.Code)
		}
		if errBody := decodeErrEnvelope(t, rec); errBody.Code != "order_not_found" {
			t.Fatalf("error code = %q; want order_not_found", errBody.Code)
		}
	})

	t.Run("checkout validates missing token, invalid JSON, and unsupported methods", func(t *testing.T) {
		h := &Handler{app: app.New(app.Dependencies{})}
		getRec := httptest.NewRecorder()
		getReq := httptest.NewRequest(http.MethodGet, apiPrefix+"/checkout", nil)
		getReq = getReq.WithContext(auth.WithUser(getReq.Context(), &auth.User{ID: 7, Role: domain.RoleUser}))
		h.checkout(getRec, getReq)
		if getRec.Code != http.StatusBadRequest {
			t.Fatalf("missing token status = %d; want 400", getRec.Code)
		}
		if errBody := decodeErrEnvelope(t, getRec); errBody.Code != "checkout_token_required" {
			t.Fatalf("error code = %q; want checkout_token_required", errBody.Code)
		}

		delRec := httptest.NewRecorder()
		delReq := httptest.NewRequest(http.MethodDelete, apiPrefix+"/checkout", nil)
		delReq = delReq.WithContext(auth.WithUser(delReq.Context(), &auth.User{ID: 7, Role: domain.RoleUser}))
		h.checkout(delRec, delReq)
		if delRec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("DELETE status = %d; want 405", delRec.Code)
		}
	})
}

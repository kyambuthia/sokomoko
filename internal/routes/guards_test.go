package routes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kyambuthia/sokomoko/internal/app"
	"github.com/kyambuthia/sokomoko/internal/auth"
	"github.com/kyambuthia/sokomoko/internal/domain"
	logisticsvc "github.com/kyambuthia/sokomoko/internal/service/logistics"
	paymentsvc "github.com/kyambuthia/sokomoko/internal/service/payment"
)

type stubRoutesAuth struct {
	app.AuthService
	user *auth.User
}

func (s *stubRoutesAuth) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.user != nil {
			r = r.WithContext(auth.WithUser(r.Context(), s.user))
		}
		next.ServeHTTP(w, r)
	})
}

type stubRoutesLogistics struct {
	app.LogisticsService
	jobsByViewFn func(context.Context, *logisticsvc.Actor, string) ([]logisticsvc.CourierDelivery, error)
}

func (s *stubRoutesLogistics) JobsByView(ctx context.Context, actor *logisticsvc.Actor, view string) ([]logisticsvc.CourierDelivery, error) {
	return s.jobsByViewFn(ctx, actor, view)
}

type stubRoutesPayment struct {
	app.PaymentService
	statusFn func(context.Context, int, int) (paymentsvc.StatusView, error)
}

func (s *stubRoutesPayment) Status(ctx context.Context, userID, orderID int) (paymentsvc.StatusView, error) {
	return s.statusFn(ctx, userID, orderID)
}

func TestRouteRoleGuards(t *testing.T) {
	okNext := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	t.Run("withRole enforces required role", func(t *testing.T) {
		denied := withRole(app.New(app.Dependencies{Auth: &stubRoutesAuth{user: &auth.User{ID: 1, Role: domain.RoleUser}}}), domain.RolePartner, okNext)
		rec := httptest.NewRecorder()
		denied.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/services", nil))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d; want 403", rec.Code)
		}

		allowed := withRole(app.New(app.Dependencies{Auth: &stubRoutesAuth{user: &auth.User{ID: 2, Role: domain.RolePartner}}}), domain.RolePartner, okNext)
		rec = httptest.NewRecorder()
		allowed.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/services", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d; want 200", rec.Code)
		}
	})

	t.Run("withAnyRole enforces any allowed role", func(t *testing.T) {
		roles := []string{domain.RoleAdmin, domain.RoleStaff}
		denied := withAnyRole(app.New(app.Dependencies{Auth: &stubRoutesAuth{user: &auth.User{ID: 1, Role: domain.RoleCourier}}}), roles, okNext)
		rec := httptest.NewRecorder()
		denied.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d; want 403", rec.Code)
		}

		allowed := withAnyRole(app.New(app.Dependencies{Auth: &stubRoutesAuth{user: &auth.User{ID: 3, Role: domain.RoleStaff}}}), roles, okNext)
		rec = httptest.NewRecorder()
		allowed.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d; want 200", rec.Code)
		}
	})
}

func TestCourierDeliveriesAndPaymentStatusGuards(t *testing.T) {
	t.Run("CourierDeliveries rejects non-GET and invalid view", func(t *testing.T) {
		a := app.New(app.Dependencies{
			Logistics: &stubRoutesLogistics{
				jobsByViewFn: func(_ context.Context, _ *logisticsvc.Actor, view string) ([]logisticsvc.CourierDelivery, error) {
					if view == "bad" {
						return nil, domain.ErrInvalidCourierView
					}
					return nil, nil
				},
			},
		})
		handler := CourierDeliveries(a)

		postRec := httptest.NewRecorder()
		handler.ServeHTTP(postRec, httptest.NewRequest(http.MethodPost, "/deliveries", nil))
		if postRec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("POST status = %d; want 405", postRec.Code)
		}

		badRec := httptest.NewRecorder()
		badReq := httptest.NewRequest(http.MethodGet, "/deliveries?view=bad", nil)
		badReq = badReq.WithContext(auth.WithUser(badReq.Context(), &auth.User{ID: 4, Role: domain.RoleCourier}))
		handler.ServeHTTP(badRec, badReq)
		if badRec.Code != http.StatusBadRequest {
			t.Fatalf("invalid view status = %d; want 400", badRec.Code)
		}
	})

	t.Run("PaymentPage returns 404 when payment is missing", func(t *testing.T) {
		a := app.New(app.Dependencies{
			Payment: &stubRoutesPayment{
				statusFn: func(context.Context, int, int) (paymentsvc.StatusView, error) {
					return paymentsvc.StatusView{}, domain.ErrPaymentNotFound
				},
			},
		})
		handler := PaymentPage(a)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/orders/42/payment", nil)
		req.SetPathValue("id", "42")
		req = req.WithContext(auth.WithUser(req.Context(), &auth.User{ID: 5, Role: domain.RoleUser}))
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d; want 404", rec.Code)
		}
	})
}

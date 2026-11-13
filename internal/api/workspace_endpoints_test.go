package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kyambuthia/sokomoko/internal/app"
	"github.com/kyambuthia/sokomoko/internal/auth"
	"github.com/kyambuthia/sokomoko/internal/domain"
	adminsvc "github.com/kyambuthia/sokomoko/internal/service/admin"
	commerceSvc "github.com/kyambuthia/sokomoko/internal/service/commerce"
	messagingSvc "github.com/kyambuthia/sokomoko/internal/service/messaging"
	partnersvc "github.com/kyambuthia/sokomoko/internal/service/partner"
)

type stubCommerceService struct {
	app.CommerceService
	cartFn           func(context.Context, int) (commerceSvc.Cart, error)
	addToCartFn      func(context.Context, int, int, int) error
	updateCartItemFn func(context.Context, int, int, int) error
	removeCartItemFn func(context.Context, int, int) error
}

func (s *stubCommerceService) Cart(ctx context.Context, userID int) (commerceSvc.Cart, error) {
	return s.cartFn(ctx, userID)
}

func (s *stubCommerceService) AddToCart(ctx context.Context, userID, productID, qty int) error {
	return s.addToCartFn(ctx, userID, productID, qty)
}

func (s *stubCommerceService) UpdateCartItem(ctx context.Context, userID, productID, qty int) error {
	return s.updateCartItemFn(ctx, userID, productID, qty)
}

func (s *stubCommerceService) RemoveFromCart(ctx context.Context, userID, productID int) error {
	return s.removeCartItemFn(ctx, userID, productID)
}

type stubMessagingService struct {
	app.MessagingService
	inboxFn func(context.Context, *messagingSvc.Actor, int) (messagingSvc.ChatInbox, error)
	sendFn  func(context.Context, *messagingSvc.Actor, int, string, string) (int, error)
}

func (s *stubMessagingService) Inbox(ctx context.Context, actor *messagingSvc.Actor, before int) (messagingSvc.ChatInbox, error) {
	return s.inboxFn(ctx, actor, before)
}

func (s *stubMessagingService) Send(ctx context.Context, actor *messagingSvc.Actor, id int, body, nonce string) (int, error) {
	return s.sendFn(ctx, actor, id, body, nonce)
}

type stubPartnerService struct {
	app.PartnerService
	settingsFn    func(context.Context) (*partnersvc.StoreSettings, error)
	updateOrderFn func(context.Context, *partnersvc.Actor, partnersvc.UpdateOrderInput) error
}

func (s *stubPartnerService) StoreSettings(ctx context.Context) (*partnersvc.StoreSettings, error) {
	return s.settingsFn(ctx)
}

func (s *stubPartnerService) UpdateOrder(ctx context.Context, actor *partnersvc.Actor, input partnersvc.UpdateOrderInput) error {
	return s.updateOrderFn(ctx, actor, input)
}

type stubAdminService struct {
	app.AdminService
	updateOrderFn    func(context.Context, *adminsvc.Actor, adminsvc.UpdateOrderInput) error
	deactivateUserFn func(context.Context, *adminsvc.Actor, string) error
}

func (s *stubAdminService) UpdateOrder(ctx context.Context, actor *adminsvc.Actor, input adminsvc.UpdateOrderInput) error {
	return s.updateOrderFn(ctx, actor, input)
}

func (s *stubAdminService) DeactivateUser(ctx context.Context, actor *adminsvc.Actor, userID string) error {
	return s.deactivateUserFn(ctx, actor, userID)
}

func TestCartEndpoints(t *testing.T) {
	h := &Handler{app: app.New(app.Dependencies{
		Commerce: &stubCommerceService{
			cartFn: func(context.Context, int) (commerceSvc.Cart, error) {
				return commerceSvc.Cart{ItemCount: 1}, nil
			},
			addToCartFn: func(context.Context, int, int, int) error {
				return commerceSvc.ErrOutOfStock
			},
		},
		Payment: &stubPaymentService{},
	})}

	unauthRec := httptest.NewRecorder()
	h.cart(unauthRec, httptest.NewRequest(http.MethodGet, apiPrefix+"/cart", nil))
	if unauthRec.Code != http.StatusUnauthorized {
		t.Fatalf("unauth cart status = %d; want 401", unauthRec.Code)
	}

	badItemRec := httptest.NewRecorder()
	badItemReq := httptest.NewRequest(http.MethodPatch, apiPrefix+"/cart/items/abc", nil)
	badItemReq.SetPathValue("productID", "abc")
	badItemReq = badItemReq.WithContext(auth.WithUser(badItemReq.Context(), &auth.User{ID: 3, Role: domain.RoleUser}))
	h.cartItem(badItemRec, badItemReq)
	if badItemRec.Code != http.StatusBadRequest {
		t.Fatalf("bad productID status = %d; want 400", badItemRec.Code)
	}

	stockRec := httptest.NewRecorder()
	stockReq := httptest.NewRequest(http.MethodPost, apiPrefix+"/cart/items", strings.NewReader(`{"product_id":1,"quantity":1}`))
	stockReq = stockReq.WithContext(auth.WithUser(stockReq.Context(), &auth.User{ID: 3, Role: domain.RoleUser}))
	h.cartItemCreate(stockRec, stockReq)
	if stockRec.Code != http.StatusConflict {
		t.Fatalf("out of stock status = %d; want 409", stockRec.Code)
	}
	if errBody := decodeErrEnvelope(t, stockRec); errBody.Code != "out_of_stock" {
		t.Fatalf("error code = %q; want out_of_stock", errBody.Code)
	}
}

func TestMessagingEndpoints(t *testing.T) {
	h := &Handler{app: app.New(app.Dependencies{
		Messaging: &stubMessagingService{
			inboxFn: func(context.Context, *messagingSvc.Actor, int) (messagingSvc.ChatInbox, error) {
				return messagingSvc.ChatInbox{}, nil
			},
			sendFn: func(_ context.Context, _ *messagingSvc.Actor, _ int, body, _ string) (int, error) {
				switch body {
				case "encrypt":
					return 0, domain.ErrChatEncryptionRequired
				case "limit":
					return 0, domain.ErrChatLimited
				default:
					return 0, domain.ErrChatInvalid
				}
			},
		},
	})}

	okRec := httptest.NewRecorder()
	okReq := httptest.NewRequest(http.MethodGet, apiPrefix+"/messages", nil)
	okReq = okReq.WithContext(auth.WithUser(okReq.Context(), &auth.User{ID: 8, Role: domain.RoleUser}))
	h.messages(okRec, okReq)
	if okRec.Code != http.StatusOK {
		t.Fatalf("inbox status = %d; want 200", okRec.Code)
	}

	for _, tc := range []struct {
		body     string
		wantCode int
		wantErr  string
	}{
		{"encrypt", http.StatusConflict, "encryption_required"},
		{"limit", http.StatusTooManyRequests, "rate_limited"},
		{"bad", http.StatusBadRequest, "invalid_message"},
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, apiPrefix+"/messages/10/messages", strings.NewReader(`{"body":"`+tc.body+`","nonce":"n1"}`))
		req.SetPathValue("id", "10")
		req = req.WithContext(auth.WithUser(req.Context(), &auth.User{ID: 8, Role: domain.RoleUser}))
		h.messageSend(rec, req)
		if rec.Code != tc.wantCode {
			t.Fatalf("body=%s status = %d; want %d", tc.body, rec.Code, tc.wantCode)
		}
		if errBody := decodeErrEnvelope(t, rec); errBody.Code != tc.wantErr {
			t.Fatalf("body=%s error code = %q; want %q", tc.body, errBody.Code, tc.wantErr)
		}
	}
}

func TestPartnerAndAdminEndpoints(t *testing.T) {
	t.Run("partner endpoints map unconfigured settings and invalid order transitions", func(t *testing.T) {
		h := &Handler{app: app.New(app.Dependencies{
			Partner: &stubPartnerService{
				settingsFn: func(context.Context) (*partnersvc.StoreSettings, error) {
					return nil, nil
				},
				updateOrderFn: func(context.Context, *partnersvc.Actor, partnersvc.UpdateOrderInput) error {
					return partnersvc.ErrInvalidOrderTransition
				},
			},
		})}

		settingsRec := httptest.NewRecorder()
		settingsReq := httptest.NewRequest(http.MethodGet, apiPrefix+"/partner/settings", nil)
		settingsReq = settingsReq.WithContext(auth.WithUser(settingsReq.Context(), &auth.User{ID: 9, Role: domain.RolePartner}))
		h.partnerSettings(settingsRec, settingsReq)
		if settingsRec.Code != http.StatusNotFound {
			t.Fatalf("settings status = %d; want 404", settingsRec.Code)
		}

		orderRec := httptest.NewRecorder()
		orderReq := httptest.NewRequest(http.MethodPatch, apiPrefix+"/partner/orders/5", strings.NewReader(`{"partner_status":"shipped"}`))
		orderReq.SetPathValue("id", "5")
		orderReq = orderReq.WithContext(auth.WithUser(orderReq.Context(), &auth.User{ID: 9, Role: domain.RolePartner}))
		h.partnerOrderUpdate(orderRec, orderReq)
		if orderRec.Code != http.StatusConflict {
			t.Fatalf("order update status = %d; want 409", orderRec.Code)
		}
		if errBody := decodeErrEnvelope(t, orderRec); errBody.Code != "invalid_order_transition" {
			t.Fatalf("error code = %q; want invalid_order_transition", errBody.Code)
		}
	})

	t.Run("admin endpoints enforce admin role for order update and validate user deactivation", func(t *testing.T) {
		h := &Handler{app: app.New(app.Dependencies{
			Admin: &stubAdminService{
				deactivateUserFn: func(context.Context, *adminsvc.Actor, string) error {
					return adminsvc.ErrUserNotFound
				},
			},
		})}

		staffRec := httptest.NewRecorder()
		staffReq := httptest.NewRequest(http.MethodPatch, apiPrefix+"/admin/orders/5", strings.NewReader(`{"status":"shipped"}`))
		staffReq.SetPathValue("id", "5")
		staffReq = staffReq.WithContext(auth.WithUser(staffReq.Context(), &auth.User{ID: 2, Role: domain.RoleStaff}))
		h.adminOrderUpdate(staffRec, staffReq)
		if staffRec.Code != http.StatusForbidden {
			t.Fatalf("staff order update status = %d; want 403", staffRec.Code)
		}

		badIDRec := httptest.NewRecorder()
		badIDReq := httptest.NewRequest(http.MethodPatch, apiPrefix+"/admin/orders/abc", nil)
		badIDReq.SetPathValue("id", "abc")
		badIDReq = badIDReq.WithContext(auth.WithUser(badIDReq.Context(), &auth.User{ID: 1, Role: domain.RoleAdmin}))
		h.adminOrderUpdate(badIDRec, badIDReq)
		if badIDRec.Code != http.StatusBadRequest {
			t.Fatalf("bad order id status = %d; want 400", badIDRec.Code)
		}

		deactRec := httptest.NewRecorder()
		deactReq := httptest.NewRequest(http.MethodDelete, apiPrefix+"/admin/team/99/deactivate", nil)
		deactReq.SetPathValue("id", "99")
		deactReq = deactReq.WithContext(auth.WithUser(deactReq.Context(), &auth.User{ID: 1, Role: domain.RoleAdmin}))
		h.adminTeamDeactivate(deactRec, deactReq)
		if deactRec.Code != http.StatusNotFound {
			t.Fatalf("deactivate missing user status = %d; want 404", deactRec.Code)
		}
	})
}

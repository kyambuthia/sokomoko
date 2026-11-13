package partner

import (
	"errors"
	"testing"

	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/domain"
)

func TestSaveStoreSettings_Validation(t *testing.T) {
	svc := newWithStore(nil)

	cases := []struct {
		input StoreSettingsInput
		want  error
	}{
		{StoreSettingsInput{}, ErrMissingStoreFields},
		{StoreSettingsInput{StoreName: "S", StoreSlug: "bad slug", ContactEmail: "a@b.co"}, ErrInvalidStoreSlug},
		{StoreSettingsInput{StoreName: "S", StoreSlug: "good", ContactEmail: "not-an-email"}, ErrInvalidContactEmail},
	}
	staff := &Actor{ID: 1, Role: "staff"}
	for _, tc := range cases {
		if _, err := svc.SaveStoreSettings(t.Context(), staff, tc.input); !errors.Is(err, tc.want) {
			t.Errorf("SaveStoreSettings(%+v) = %v, want %v", tc.input, err, tc.want)
		}
	}
}

func TestSaveStoreSettings_RequiresPlatformRole(t *testing.T) {
	svc := newWithStore(nil)
	valid := StoreSettingsInput{StoreName: "S", StoreSlug: "good", ContactEmail: "a@b.co"}
	for _, actor := range []*Actor{nil, {ID: 7, Role: "partner"}, {ID: 8, Role: "user"}} {
		if _, err := svc.SaveStoreSettings(t.Context(), actor, valid); !errors.Is(err, ErrForbiddenSettingsUpdate) {
			t.Errorf("SaveStoreSettings(%+v) = %v, want ErrForbiddenSettingsUpdate", actor, err)
		}
	}
}

func TestUpdateOrder_RequiresStaff(t *testing.T) {
	svc := newWithStore(nil)
	if err := svc.UpdateOrder(t.Context(), &Actor{ID: 1, Role: "user"}, UpdateOrderInput{OrderID: "1"}); !errors.Is(err, ErrForbiddenOrderUpdate) {
		t.Fatalf("err = %v", err)
	}
	if err := svc.UpdateOrder(t.Context(), &Actor{ID: 1, Role: "staff"}, UpdateOrderInput{OrderID: "x"}); !errors.Is(err, ErrInvalidOrderID) {
		t.Fatalf("err = %v", err)
	}
}

func TestCourierCompletedOrdersUseCompletedFilterAndSummary(t *testing.T) {
	delivered := db.Order{Status: db.OrderStatusDelivered, PartnerStatus: "dispatched"}
	if !matchesOrderFilter(delivered, "completed") || matchesOrderFilter(delivered, "dispatched") {
		t.Fatal("courier delivered order appears in the wrong queue")
	}
	summary := summaryForOrders([]Order{
		{Status: db.OrderStatusDelivered, PartnerStatus: "dispatched"},
		{Status: db.OrderStatusShipped, PartnerStatus: "dispatched"},
		{Status: db.OrderStatusCancelled, PartnerStatus: "dispatched"},
	})
	if summary.CompletedCount != 1 || summary.DispatchedCount != 1 {
		t.Fatalf("summary=%+v", summary)
	}
}

func TestWorkspaceRejectsUntrustedActorsBeforeStoreAccess(t *testing.T) {
	svc := newWithStore(nil) // Any accidental store access panics rather than hiding a leak.
	actors := []*Actor{nil, {ID: 1, Role: domain.RoleUser}, {ID: 1, Role: domain.RoleCourier}, {ID: 1, Role: "unknown"}, {ID: 0, Role: domain.RoleAdmin}, {ID: -1, Role: domain.RoleStaff}, {ID: 0, Role: domain.RolePartner}}
	for _, actor := range actors {
		calls := []struct {
			name string
			run  func() error
			want error
		}{
			{"dashboard", func() error { _, err := svc.DashboardForActor(t.Context(), actor); return err }, ErrForbiddenWorkspace},
			{"products", func() error { _, err := svc.ProductsForActor(t.Context(), actor); return err }, ErrForbiddenWorkspace},
			{"products page", func() error { _, _, err := svc.ProductsPageForActor(t.Context(), actor, 1, 20); return err }, ErrForbiddenWorkspace},
			{"orders", func() error { _, err := svc.OrdersForActor(t.Context(), actor, "all"); return err }, ErrForbiddenWorkspace},
			{"orders page", func() error { _, _, err := svc.OrdersPageForActor(t.Context(), actor, "all", 1, 20); return err }, ErrForbiddenWorkspace},
			{"create product", func() error { return svc.CreateProductForActor(t.Context(), actor, CreateProductInput{}) }, ErrForbiddenWorkspace},
			{"edit product", func() error { _, _, err := svc.ProductForEdit(t.Context(), actor, "1"); return err }, ErrProductNotFound},
			{"update order", func() error { return svc.UpdateOrder(t.Context(), actor, UpdateOrderInput{OrderID: "1"}) }, ErrForbiddenOrderUpdate},
			{"settings", func() error { _, err := svc.SaveStoreSettings(t.Context(), actor, StoreSettingsInput{}); return err }, ErrForbiddenSettingsUpdate},
		}
		for _, call := range calls {
			if err := call.run(); !errors.Is(err, call.want) {
				t.Errorf("%s actor=%+v: %v, want %v", call.name, actor, err, call.want)
			}
		}
	}
}

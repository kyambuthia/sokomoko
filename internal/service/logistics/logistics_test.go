package logistics

import (
	"errors"
	"testing"

	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/domain"
)

func TestDispatchRejectsNonOperatorsBeforeStoreAccess(t *testing.T) {
	svc := New(nil)
	for _, actor := range []*Actor{nil, {ID: 1, Role: domain.RoleUser}, {ID: 1, Role: domain.RolePartner}, {ID: 1, Role: domain.RoleCourier}, {ID: 1, Role: "unknown"}, {ID: 0, Role: domain.RoleAdmin}, {ID: -1, Role: domain.RoleStaff}} {
		if _, err := svc.DispatchBoard(t.Context(), actor); !errors.Is(err, ErrForbidden) {
			t.Errorf("dispatch actor=%+v: %v", actor, err)
		}
		if err := svc.Assign(t.Context(), actor, "1", "2"); !errors.Is(err, ErrForbidden) {
			t.Errorf("assign actor=%+v: %v", actor, err)
		}
	}
}

func TestCourierRejectsOtherRolesBeforeStoreAccess(t *testing.T) {
	svc := New(nil)
	for _, actor := range []*Actor{nil, {ID: 1, Role: domain.RoleUser}, {ID: 1, Role: domain.RolePartner}, {ID: 1, Role: domain.RoleAdmin}, {ID: 1, Role: domain.RoleStaff}, {ID: 1, Role: "unknown"}, {ID: 0, Role: domain.RoleCourier}} {
		if _, err := svc.JobsByView(t.Context(), actor, "active"); !errors.Is(err, ErrForbidden) {
			t.Errorf("filtered jobs actor=%+v: %v", actor, err)
		}
		if _, err := svc.Jobs(t.Context(), actor); !errors.Is(err, ErrForbidden) {
			t.Errorf("jobs actor=%+v: %v", actor, err)
		}
		if err := svc.Update(t.Context(), actor, "1", db.CourierDeliveryDelivered, ""); !errors.Is(err, ErrForbidden) {
			t.Errorf("update actor=%+v: %v", actor, err)
		}
	}
}

func TestCourierRejectsInvalidViewBeforeStoreAccess(t *testing.T) {
	_, err := New(nil).JobsByView(t.Context(), &Actor{ID: 1, Role: domain.RoleCourier}, "unknown")
	if !errors.Is(err, db.ErrInvalidCourierView) {
		t.Fatal(err)
	}
}

package logistics

import (
	"errors"
	"testing"

	"github.com/kyambuthia/sokomoko/internal/db"
)

func TestDispatchRejectsNonOperatorsBeforeStoreAccess(t *testing.T) {
	svc := New(nil)
	for _, actor := range []*Actor{nil, {ID: 1, Role: db.RoleUser}, {ID: 1, Role: db.RolePartner}, {ID: 1, Role: db.RoleCourier}, {ID: 1, Role: "unknown"}, {ID: 0, Role: db.RoleAdmin}, {ID: -1, Role: db.RoleStaff}} {
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
	for _, actor := range []*Actor{nil, {ID: 1, Role: db.RoleUser}, {ID: 1, Role: db.RolePartner}, {ID: 1, Role: db.RoleAdmin}, {ID: 1, Role: db.RoleStaff}, {ID: 1, Role: "unknown"}, {ID: 0, Role: db.RoleCourier}} {
		if _, err := svc.Jobs(t.Context(), actor); !errors.Is(err, ErrForbidden) {
			t.Errorf("jobs actor=%+v: %v", actor, err)
		}
		if err := svc.Update(t.Context(), actor, "1", db.CourierDeliveryDelivered, ""); !errors.Is(err, ErrForbidden) {
			t.Errorf("update actor=%+v: %v", actor, err)
		}
	}
}

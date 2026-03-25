package admin

import (
	"errors"
	"testing"

	"github.com/kyambuthia/sokomoko/internal/db"
)

func TestUpdateOrder_NonAdmin_ReturnsForbidden(t *testing.T) {
	svc := New(nil)

	err := svc.UpdateOrder(nil, UpdateOrderInput{OrderID: "1"})
	if !errors.Is(err, ErrForbiddenOrderUpdate) {
		t.Fatalf("expected ErrForbiddenOrderUpdate, got %v", err)
	}
}

func TestDeactivateUser_InvalidID_ReturnsError(t *testing.T) {
	svc := New(nil)

	err := svc.DeactivateUser(&db.User{ID: 1, Role: "admin"}, "abc")
	if !errors.Is(err, ErrInvalidUserID) {
		t.Fatalf("expected ErrInvalidUserID, got %v", err)
	}
}

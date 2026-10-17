package admin

import (
	"errors"
	"testing"

	"github.com/kyambuthia/sokomoko/internal/db"
)

func TestUpdateOrder_NonAdmin_ReturnsForbidden(t *testing.T) {
	svc := newWithStore(nil)

	err := svc.UpdateOrder(nil, UpdateOrderInput{OrderID: "1"})
	if !errors.Is(err, ErrForbiddenOrderUpdate) {
		t.Fatalf("expected ErrForbiddenOrderUpdate, got %v", err)
	}
}

func TestDeactivateUser_InvalidID_ReturnsError(t *testing.T) {
	svc := newWithStore(nil)

	err := svc.DeactivateUser(&Actor{ID: 1, Role: "admin"}, "abc")
	if !errors.Is(err, ErrInvalidUserID) {
		t.Fatalf("expected ErrInvalidUserID, got %v", err)
	}
}

// ordersStore serves a fixed order list; other store methods are not used.
type ordersStore struct {
	store
	orders []db.Order
}

func (s ordersStore) ListAllOrders() ([]db.Order, error) { return s.orders, nil }

func TestDeliveries_CountsOpenOrdersAndListsOldestFirst(t *testing.T) {
	// ListAllOrders returns newest first.
	svc := newWithStore(ordersStore{orders: []db.Order{
		{ID: 5, Status: "processing", DeliveryStatus: "processing"},
		{ID: 4, Status: "cancelled", DeliveryStatus: "queued"},
		{ID: 3, Status: "delivered", DeliveryStatus: "delivered"},
		{ID: 2, Status: "shipped", DeliveryStatus: "shipped"},
		{ID: 1, Status: "pending", DeliveryStatus: "queued"},
	}})
	board, err := svc.Deliveries()
	if err != nil {
		t.Fatal(err)
	}
	if board.QueuedCount != 1 || board.ProcessingCount != 1 || board.ShippedCount != 1 || board.DeliveredCount != 1 {
		t.Fatalf("counts = %+v", board)
	}
	var ids []int
	for _, order := range board.Pending {
		ids = append(ids, order.ID)
	}
	if len(ids) != 3 || ids[0] != 1 || ids[1] != 2 || ids[2] != 5 {
		t.Fatalf("pending ids = %v, want [1 2 5]", ids)
	}
}

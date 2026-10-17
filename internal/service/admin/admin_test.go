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

// categoryStore records category writes.
type categoryStore struct {
	store
	created []db.Category
	err     error
}

func (s *categoryStore) CreateCategory(c db.Category) (int64, error) {
	s.created = append(s.created, c)
	return 1, s.err
}

func (s *categoryStore) CreateAuditLog(int, string, string, int, string) error { return nil }

func TestCreateCategory(t *testing.T) {
	staff := &Actor{ID: 2, Role: "staff"}
	fake := &categoryStore{}
	svc := newWithStore(fake)

	if err := svc.CreateCategory(&Actor{ID: 3, Role: "partner"}, CategoryInput{Name: "X"}); !errors.Is(err, ErrForbiddenCatalogAction) {
		t.Fatalf("partner create = %v", err)
	}
	for _, input := range []CategoryInput{{}, {Name: "Ok", Slug: "Not Valid"}, {Name: "!!!"}} {
		if err := svc.CreateCategory(staff, input); !errors.Is(err, ErrInvalidCategory) {
			t.Errorf("CreateCategory(%+v) = %v, want ErrInvalidCategory", input, err)
		}
	}
	if err := svc.CreateCategory(staff, CategoryInput{Name: " Home & Garden "}); err != nil {
		t.Fatal(err)
	}
	if len(fake.created) != 1 || fake.created[0].Slug != "home-garden" || fake.created[0].Name != "Home & Garden" {
		t.Fatalf("created = %+v", fake.created)
	}

	fake.err = db.ErrUniqueConstraint
	if err := svc.CreateCategory(staff, CategoryInput{Name: "Again"}); !errors.Is(err, ErrDuplicateCategory) {
		t.Fatalf("duplicate = %v", err)
	}
}

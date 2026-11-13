package account

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/money"
)

type mockAccountStore struct {
	getOrderForUserFn       func(ctx context.Context, userID, orderID int) (*db.Order, error)
	listOrdersByUserPageFn  func(ctx context.Context, userID, limit, offset int) ([]db.Order, int, error)
	cancelOrderByCustomerFn func(ctx context.Context, userID, orderID int) error
}

func (m *mockAccountStore) GetOrderForUser(ctx context.Context, userID, orderID int) (*db.Order, error) {
	if m.getOrderForUserFn != nil {
		return m.getOrderForUserFn(ctx, userID, orderID)
	}
	return nil, nil
}

func (m *mockAccountStore) ListOrdersByUserPage(ctx context.Context, userID, limit, offset int) ([]db.Order, int, error) {
	if m.listOrdersByUserPageFn != nil {
		return m.listOrdersByUserPageFn(ctx, userID, limit, offset)
	}
	return nil, 0, nil
}

func (m *mockAccountStore) CancelOrderByCustomer(ctx context.Context, userID, orderID int) error {
	if m.cancelOrderByCustomerFn != nil {
		return m.cancelOrderByCustomerFn(ctx, userID, orderID)
	}
	return nil
}

func TestCancelOrder(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid IDs return ErrOrderNotFound without touching store", func(t *testing.T) {
		svc := New(&mockAccountStore{
			cancelOrderByCustomerFn: func(ctx context.Context, userID, orderID int) error {
				t.Fatal("store should not be called for invalid IDs")
				return nil
			},
		})

		if err := svc.CancelOrder(ctx, 0, 10); !errors.Is(err, ErrOrderNotFound) {
			t.Fatalf("expected ErrOrderNotFound, got %v", err)
		}
		if err := svc.CancelOrder(ctx, 5, 0); !errors.Is(err, ErrOrderNotFound) {
			t.Fatalf("expected ErrOrderNotFound, got %v", err)
		}
		if err := svc.CancelOrder(ctx, -1, -2); !errors.Is(err, ErrOrderNotFound) {
			t.Fatalf("expected ErrOrderNotFound, got %v", err)
		}
	})

	t.Run("translates db error types", func(t *testing.T) {
		svcNotFound := New(&mockAccountStore{
			cancelOrderByCustomerFn: func(ctx context.Context, userID, orderID int) error {
				return db.ErrOrderNotFound
			},
		})
		if err := svcNotFound.CancelOrder(ctx, 1, 2); !errors.Is(err, ErrOrderNotFound) {
			t.Fatalf("expected ErrOrderNotFound, got %v", err)
		}

		svcNotCancellable := New(&mockAccountStore{
			cancelOrderByCustomerFn: func(ctx context.Context, userID, orderID int) error {
				return db.ErrOrderNotCancellable
			},
		})
		if err := svcNotCancellable.CancelOrder(ctx, 1, 2); !errors.Is(err, ErrOrderNotCancellable) {
			t.Fatalf("expected ErrOrderNotCancellable, got %v", err)
		}

		customErr := errors.New("db disconnect")
		svcCustom := New(&mockAccountStore{
			cancelOrderByCustomerFn: func(ctx context.Context, userID, orderID int) error {
				return customErr
			},
		})
		if err := svcCustom.CancelOrder(ctx, 1, 2); !errors.Is(err, customErr) {
			t.Fatalf("expected customErr, got %v", err)
		}
	})

	t.Run("success", func(t *testing.T) {
		var calledUser, calledOrder int
		svc := New(&mockAccountStore{
			cancelOrderByCustomerFn: func(ctx context.Context, userID, orderID int) error {
				calledUser = userID
				calledOrder = orderID
				return nil
			},
		})

		if err := svc.CancelOrder(ctx, 42, 99); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if calledUser != 42 || calledOrder != 99 {
			t.Fatalf("called with %d, %d; want 42, 99", calledUser, calledOrder)
		}
	})
}

func TestOrderForUser(t *testing.T) {
	ctx := context.Background()

	t.Run("returns nil on nil store result or error", func(t *testing.T) {
		svc := New(&mockAccountStore{
			getOrderForUserFn: func(ctx context.Context, userID, orderID int) (*db.Order, error) {
				return nil, nil
			},
		})
		order, err := svc.OrderForUser(ctx, 1, 10)
		if err != nil || order != nil {
			t.Fatalf("expected nil, nil; got %v, %v", order, err)
		}

		boomErr := errors.New("query failed")
		svcErr := New(&mockAccountStore{
			getOrderForUserFn: func(ctx context.Context, userID, orderID int) (*db.Order, error) {
				return nil, boomErr
			},
		})
		order, err = svcErr.OrderForUser(ctx, 1, 10)
		if !errors.Is(err, boomErr) || order != nil {
			t.Fatalf("expected boomErr, got %v, %v", order, err)
		}
	})

	t.Run("maps fields, cancellable state and items correctly", func(t *testing.T) {
		now := time.Now()
		svc := New(&mockAccountStore{
			getOrderForUserFn: func(ctx context.Context, userID, orderID int) (*db.Order, error) {
				return &db.Order{
					ID:              orderID,
					UserID:          userID,
					Currency:        "KES",
					Status:          db.OrderStatusPending,
					PartnerStatus:   "new",
					DeliveryStatus:  "unassigned",
					DeliveryNotice:  "Ring bell",
					DeliveryAddress: "123 Elm St",
					Subtotal:        money.Cents(1000),
					ShippingFee:     money.Cents(200),
					TaxAmount:       money.Cents(160),
					TotalAmount:     money.Cents(1360),
					CreatedAt:       now,
					UpdatedAt:       now,
					PaymentMethod:   "cod",
					PaymentStatus:   "pending",
					Items: []db.OrderItem{
						{
							ProductID:   7,
							ProductName: "Test Widget",
							Quantity:    2,
							UnitPrice:   money.Cents(500),
							LineTotal:   money.Cents(1000),
						},
					},
				}, nil
			},
		})

		order, err := svc.OrderForUser(ctx, 10, 20)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if order == nil {
			t.Fatal("expected order, got nil")
		}
		if order.ID != 20 || order.Currency != "KES" || order.TotalAmount != money.Cents(1360) {
			t.Fatalf("order mapped incorrectly: %+v", order)
		}
		if !order.Cancellable {
			t.Fatal("pending new order without captured payment should be cancellable")
		}
		if len(order.Items) != 1 || order.Items[0].ProductName != "Test Widget" {
			t.Fatalf("items mapped incorrectly: %+v", order.Items)
		}
	})
}

func TestOrdersForUserPage(t *testing.T) {
	ctx := context.Background()

	t.Run("calculates offset and maps items", func(t *testing.T) {
		var capturedLimit, capturedOffset int
		svc := New(&mockAccountStore{
			listOrdersByUserPageFn: func(ctx context.Context, userID, limit, offset int) ([]db.Order, int, error) {
				capturedLimit = limit
				capturedOffset = offset
				return []db.Order{
					{
						ID:            101,
						Status:        db.OrderStatusProcessing,
						PartnerStatus: "accepted",
						PaymentStatus: db.PaymentStatusCaptured,
					},
				}, 15, nil
			},
		})

		orders, total, err := svc.OrdersForUserPage(ctx, 5, 3, 5) // page 3, limit 5 -> offset 10
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if total != 15 {
			t.Fatalf("expected total 15, got %d", total)
		}
		if capturedLimit != 5 || capturedOffset != 10 {
			t.Fatalf("expected limit 5 offset 10, got limit %d offset %d", capturedLimit, capturedOffset)
		}
		if len(orders) != 1 {
			t.Fatalf("expected 1 order, got %d", len(orders))
		}
		if orders[0].Cancellable {
			t.Fatal("accepted order with captured payment must not be cancellable")
		}
	})

	t.Run("propagates error", func(t *testing.T) {
		errBoom := errors.New("timeout")
		svc := New(&mockAccountStore{
			listOrdersByUserPageFn: func(ctx context.Context, userID, limit, offset int) ([]db.Order, int, error) {
				return nil, 0, errBoom
			},
		})

		_, _, err := svc.OrdersForUserPage(ctx, 1, 1, 10)
		if !errors.Is(err, errBoom) {
			t.Fatalf("expected errBoom, got %v", err)
		}
	})
}

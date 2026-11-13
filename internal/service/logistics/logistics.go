// Package logistics coordinates dispatcher assignment and courier delivery
// updates. Rider queries are scoped to the signed-in courier's user ID.
package logistics

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/domain"
)

var (
	ErrForbidden           = errors.New("forbidden logistics action")
	ErrInvalidOrderID      = errors.New("invalid order id")
	ErrInvalidCourierID    = errors.New("invalid courier id")
	ErrInvalidDeliveryID   = errors.New("invalid delivery id")
	ErrCourierNotFound     = errors.New("courier account not found")
	ErrOrderNotReady       = errors.New("order is not ready for courier assignment")
	ErrDeliveryStarted     = errors.New("delivery has already been picked up")
	ErrDeliveryNotFound    = errors.New("delivery not found")
	ErrInvalidTransition   = errors.New("invalid delivery update")
	ErrFailureNoteRequired = errors.New("a note is required when delivery fails")
)

var failureReasons = map[string]bool{
	"Recipient unavailable": true,
	"Address issue":         true,
	"Item damaged":          true,
	"Recipient declined":    true,
	"Other":                 true,
}

type Actor struct {
	ID   int
	Role string
}

type store interface {
	ListCourierUsers(context.Context) ([]db.CourierUser, error)
	ListCourierDispatchOrders(context.Context) ([]db.CourierDelivery, error)
	ListCourierDeliveriesByView(context.Context, int, string) ([]db.CourierDelivery, error)
	AssignCourierToOrder(context.Context, int, int, int) error
	UpdateCourierDelivery(context.Context, int, int, string, string) error
}

var _ store = (*db.Store)(nil)

type Service struct {
	store store
}

func New(s store) *Service { return &Service{store: s} }

type DispatchBoard struct {
	Couriers []db.CourierUser
	Orders   []db.CourierDelivery
}

func (s *Service) DispatchBoard(ctx context.Context, actor *Actor) (DispatchBoard, error) {
	if !canDispatch(actor) {
		return DispatchBoard{}, ErrForbidden
	}
	couriers, err := s.store.ListCourierUsers(ctx)
	if err != nil {
		return DispatchBoard{}, err
	}
	orders, err := s.store.ListCourierDispatchOrders(ctx)
	if err != nil {
		return DispatchBoard{}, err
	}
	return DispatchBoard{Couriers: couriers, Orders: orders}, nil
}

func (s *Service) Assign(ctx context.Context, actor *Actor, orderIDRaw, courierIDRaw string) error {
	if !canDispatch(actor) {
		return ErrForbidden
	}
	orderID, ok := parsePositiveID(orderIDRaw)
	if !ok {
		return ErrInvalidOrderID
	}
	courierID, ok := parsePositiveID(courierIDRaw)
	if !ok {
		return ErrInvalidCourierID
	}
	if err := s.store.AssignCourierToOrder(ctx, orderID, courierID, actor.ID); err != nil {
		switch {
		case errors.Is(err, db.ErrCourierDispatchForbidden):
			return ErrForbidden
		case errors.Is(err, db.ErrCourierUserNotFound):
			return ErrCourierNotFound
		case errors.Is(err, db.ErrDeliveryNotReady):
			return ErrOrderNotReady
		case errors.Is(err, db.ErrDeliveryAlreadyStarted):
			return ErrDeliveryStarted
		case errors.Is(err, db.ErrOrderNotFound):
			return ErrOrderNotReady
		default:
			return err
		}
	}
	return nil
}

func (s *Service) Jobs(ctx context.Context, actor *Actor) ([]db.CourierDelivery, error) {
	if actor == nil || actor.ID <= 0 || actor.Role != domain.RoleCourier {
		return nil, ErrForbidden
	}
	return s.JobsByView(ctx, actor, "all")
}

func (s *Service) JobsByView(ctx context.Context, actor *Actor, view string) ([]db.CourierDelivery, error) {
	if actor == nil || actor.ID <= 0 || actor.Role != domain.RoleCourier {
		return nil, ErrForbidden
	}
	if view == "" {
		view = "all"
	}
	if view != "all" && view != "active" && view != "completed" {
		return nil, db.ErrInvalidCourierView
	}
	return s.store.ListCourierDeliveriesByView(ctx, actor.ID, view)
}

func (s *Service) Update(ctx context.Context, actor *Actor, deliveryIDRaw, status, note string) error {
	if actor == nil || actor.ID <= 0 || actor.Role != domain.RoleCourier {
		return ErrForbidden
	}
	deliveryID, ok := parsePositiveID(deliveryIDRaw)
	if !ok {
		return ErrInvalidDeliveryID
	}
	status = strings.TrimSpace(status)
	note = strings.TrimSpace(note)
	if status == db.CourierDeliveryFailed {
		if !failureReasons[note] {
			return ErrFailureNoteRequired
		}
	} else {
		note = ""
	}
	if err := s.store.UpdateCourierDelivery(ctx, deliveryID, actor.ID, status, note); err != nil {
		switch {
		case errors.Is(err, db.ErrCourierDeliveryNotFound):
			return ErrDeliveryNotFound
		case errors.Is(err, db.ErrCourierDeliveryForbidden):
			return ErrForbidden
		case errors.Is(err, db.ErrInvalidCourierTransition):
			return ErrInvalidTransition
		default:
			return err
		}
	}
	return nil
}

func canDispatch(actor *Actor) bool {
	return actor != nil && actor.ID > 0 && (actor.Role == domain.RoleAdmin || actor.Role == domain.RoleStaff)
}

func parsePositiveID(raw string) (int, bool) {
	id, err := strconv.Atoi(strings.TrimSpace(raw))
	return id, err == nil && id > 0
}

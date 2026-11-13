// Package partner backs the partner workspace: store setup, catalog entry, and
// order fulfillment.
package partner

import (
	"context"
	"errors"

	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/domain"
)

var (
	ErrForbiddenWorkspace        = errors.New("forbidden partner workspace")
	ErrStoreNotConfigured        = errors.New("store not configured")
	ErrMissingStoreFields        = errors.New("missing store fields")
	ErrInvalidStoreSlug          = errors.New("invalid store slug")
	ErrInvalidContactEmail       = errors.New("invalid contact email")
	ErrForbiddenOrderUpdate      = errors.New("forbidden order update")
	ErrForbiddenSettingsUpdate   = errors.New("forbidden store settings update")
	ErrInvalidOrderID            = errors.New("invalid order id")
	ErrOrderNotFound             = errors.New("order not found")
	ErrInvalidOrderTransition    = errors.New("invalid order transition")
	ErrMissingProductFields      = errors.New("missing product fields")
	ErrInvalidProductPrice       = errors.New("invalid product price")
	ErrInvalidProductStock       = errors.New("invalid product stock")
	ErrInvalidProductImage       = db.ErrInvalidProductImage
	ErrInvalidProductVideo       = db.ErrInvalidProductVideo
	ErrInvalidProductMedia       = db.ErrInvalidProductMedia
	ErrUnableToCreateProduct     = errors.New("unable to create product")
	ErrUnableToCreateProductSlug = errors.New("unable to create product slug")
	ErrProductNotFound           = errors.New("product not found")
)

type Service struct {
	store store
}

type Actor struct {
	ID   int
	Role string
}

func New(store *db.Store) *Service {
	return &Service{store: store}
}

func newWithStore(store store) *Service {
	return &Service{store: store}
}

func (s *Service) partnerIDForActor(ctx context.Context, actor *Actor) (int, error) {
	if !canUseWorkspace(actor) {
		return 0, ErrForbiddenWorkspace
	}
	if !actorIsPartner(actor) {
		return 0, nil
	}
	return s.store.PartnerIDForUser(ctx, actor.ID)
}

func canUseWorkspace(actor *Actor) bool {
	return actor != nil && actor.ID > 0 && (actor.Role == domain.RoleAdmin || actor.Role == domain.RoleStaff || actor.Role == domain.RolePartner)
}

func actorIsPartner(actor *Actor) bool {
	return actor != nil && actor.Role == domain.RolePartner && actor.ID > 0
}

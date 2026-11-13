package partner

import (
	"context"
	"errors"
	"net/mail"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/slug"
)

type StoreSettingsInput struct {
	StoreName    string
	StoreSlug    string
	Description  string
	ContactEmail string
}

type StoreSettings struct {
	StoreName    string
	StoreSlug    string
	Description  string
	ContactEmail string
}

type DashboardData struct {
	Settings     StoreSettings
	ProductCount int
	OrderSummary Summary
}

func (s *Service) StoreSettings(ctx context.Context) (*StoreSettings, error) {
	settings, err := s.store.GetStoreSettings(ctx)
	if err != nil || settings == nil {
		return nil, err
	}
	mapped := mapStoreSettings(*settings)
	return &mapped, nil
}

// CanEditStoreSettings reports whether actor may change the marketplace-wide
// store settings. Partner accounts manage only their own catalog and orders.
func CanEditStoreSettings(actor *Actor) bool {
	return actor != nil && actor.ID > 0 && (actor.Role == db.RoleAdmin || actor.Role == db.RoleStaff)
}

func (s *Service) SaveStoreSettings(ctx context.Context, actor *Actor, input StoreSettingsInput) (StoreSettings, error) {
	if !CanEditStoreSettings(actor) {
		return StoreSettings{}, ErrForbiddenSettingsUpdate
	}
	settings := StoreSettings{
		StoreName:    strings.TrimSpace(input.StoreName),
		StoreSlug:    strings.ToLower(strings.TrimSpace(input.StoreSlug)),
		Description:  strings.TrimSpace(input.Description),
		ContactEmail: strings.ToLower(strings.TrimSpace(input.ContactEmail)),
	}
	if settings.StoreName == "" || settings.StoreSlug == "" || settings.ContactEmail == "" {
		return settings, ErrMissingStoreFields
	}
	if !slug.Valid(settings.StoreSlug) {
		return settings, ErrInvalidStoreSlug
	}
	if addr, err := mail.ParseAddress(settings.ContactEmail); err != nil || addr.Address != settings.ContactEmail {
		return settings, ErrInvalidContactEmail
	}
	err := s.store.UpsertStoreSettingsForActor(ctx, actor.ID, actor.Role, db.StoreSettings{
		StoreName:    settings.StoreName,
		StoreSlug:    settings.StoreSlug,
		Description:  settings.Description,
		ContactEmail: settings.ContactEmail,
	})
	if errors.Is(err, db.ErrWorkspaceActorNotFound) {
		return settings, ErrForbiddenSettingsUpdate
	}
	return settings, err
}

func (s *Service) DashboardForActor(ctx context.Context, actor *Actor) (DashboardData, error) {
	if !canUseWorkspace(actor) {
		return DashboardData{}, ErrForbiddenWorkspace
	}
	settings, err := s.requireStoreSettings(ctx)
	if err != nil {
		return DashboardData{}, err
	}

	partnerID, err := s.partnerIDForActor(ctx, actor)
	if err != nil {
		return DashboardData{}, err
	}
	if actorIsPartner(actor) && partnerID == 0 {
		return DashboardData{Settings: *settings}, nil
	}
	count, err := s.store.CountFilteredProducts(ctx, db.ProductFilter{PartnerID: partnerID})
	if err != nil {
		return DashboardData{}, err
	}
	summary, err := s.store.FulfillmentSummary(ctx, partnerID)
	if err != nil {
		return DashboardData{}, err
	}
	return DashboardData{
		Settings:     *settings,
		ProductCount: count,
		OrderSummary: Summary(summary),
	}, nil
}

func (s *Service) requireStoreSettings(ctx context.Context) (*StoreSettings, error) {
	settings, err := s.StoreSettings(ctx)
	if err != nil {
		return nil, err
	}
	if settings == nil {
		return nil, ErrStoreNotConfigured
	}
	return settings, nil
}

func mapStoreSettings(settings db.StoreSettings) StoreSettings {
	return StoreSettings{
		StoreName:    settings.StoreName,
		StoreSlug:    settings.StoreSlug,
		Description:  settings.Description,
		ContactEmail: settings.ContactEmail,
	}
}

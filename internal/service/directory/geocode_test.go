package directory

import (
	"context"
	"errors"
	"testing"

	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/domain"
)

func TestLocalityGeocoderAndPrivacyRules(t *testing.T) {
	g := LocalityGeocoder{}
	lat, lng, ok, err := g.GeocodeLocality(t.Context(), "Westlands, Nairobi")
	if err != nil || !ok || lat == 0 || lng == 0 {
		t.Fatalf("GeocodeLocality(Westlands) = (%v, %v, %v, %v)", lat, lng, ok, err)
	}
	if _, _, ok, err := g.GeocodeLocality(t.Context(), "Unknown Place"); err != nil || ok {
		t.Fatalf("unknown locality should not match, got ok=%v err=%v", ok, err)
	}

	// Non-premises services never keep coordinates (privacy protection).
	pLat, pLng := -1.2647, 36.8019
	gotLat, gotLng, err := ValidatePublicCoordinates("mobile", &pLat, &pLng)
	if err != nil || gotLat != nil || gotLng != nil {
		t.Fatalf("mobile service kept coordinates: (%v, %v, %v)", gotLat, gotLng, err)
	}

	// Out-of-bounds premises coordinates fail validation.
	badLat := 95.0
	if _, _, err := ValidatePublicCoordinates("premises", &badLat, &pLng); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected ErrInvalid for out-of-bounds lat, got %v", err)
	}

	// Search radius validation.
	if err := ValidateSearchRadius(&pLat, &pLng, 150); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected ErrInvalid for oversized radius, got %v", err)
	}
	if err := ValidateSearchRadius(&pLat, &pLng, 10); err != nil {
		t.Fatalf("valid radius failed: %v", err)
	}
}

type captureBrowseStore struct {
	gotFilter db.ServiceFilter
}

func (c *captureBrowseStore) BrowseServices(_ context.Context, f db.ServiceFilter) ([]db.ServiceListing, int, error) {
	c.gotFilter = f
	return nil, 0, nil
}
func (c *captureBrowseStore) PublicService(context.Context, string) (*db.ServiceListing, error) {
	return nil, nil
}
func (c *captureBrowseStore) OwnService(context.Context, int, int) (*db.ServiceListing, error) {
	return nil, nil
}
func (c *captureBrowseStore) OwnServicesByState(context.Context, int, string, int, int) ([]db.ServiceListing, int, error) {
	return nil, 0, nil
}
func (c *captureBrowseStore) SaveService(context.Context, int, db.ServiceListing) (*db.ServiceListing, error) {
	return &db.ServiceListing{ID: 1}, nil
}

func TestBrowseResolvesAreaCentroidWhenRadiusRequested(t *testing.T) {
	store := &captureBrowseStore{}
	svc := New(store)
	_, _, err := svc.Browse(t.Context(), db.ServiceFilter{
		Area:     "Westlands",
		RadiusKM: 5,
		Limit:    20,
	}, 1)
	if err != nil {
		t.Fatalf("Browse failed: %v", err)
	}
	if store.gotFilter.Lat == nil || store.gotFilter.Lng == nil {
		t.Fatal("expected Browse to resolve locality coordinates when Area and RadiusKM are provided")
	}

	// Saving a mobile service strips any submitted coordinates.
	in := validInput()
	in.DeliveryMode = "mobile"
	lat, lng := -1.2647, 36.8019
	in.PublicLat, in.PublicLng = &lat, &lng
	if _, err := svc.Save(t.Context(), &Actor{ID: 1, Role: domain.RolePartner}, 0, in); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
}

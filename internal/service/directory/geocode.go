package directory

import (
	"context"
	"math"
	"strings"
)

const MaxSearchRadiusKM = 100.0

// Geocoder resolves a named public locality or neighbourhood to approximate
// coordinates without exposing private addresses or wiring keystroke queries.
type Geocoder interface {
	GeocodeLocality(ctx context.Context, area string) (lat, lng float64, ok bool, err error)
}

// LocalityGeocoder maps known public service areas to coarse locality centroids.
type LocalityGeocoder struct{}

var localityCentroids = map[string][2]float64{
	"nairobi":    {-1.286389, 36.817223},
	"cbd":        {-1.283333, 36.816667},
	"westlands":  {-1.264700, 36.801900},
	"kilimani":   {-1.289600, 36.783100},
	"kileleshwa": {-1.281400, 36.776400},
	"lavington":  {-1.279700, 36.766700},
	"karen":      {-1.319200, 36.706900},
	"langata":    {-1.366700, 36.733300},
	"parklands":  {-1.263600, 36.818600},
	"upper hill": {-1.298100, 36.816900},
	"kasarani":   {-1.221900, 36.896900},
	"mombasa":    {-4.043477, 39.668206},
	"kisumu":     {-0.091702, 34.767956},
	"nakuru":     {-0.303099, 36.080026},
	"eldoret":    {0.514277, 35.269780},
}

func (LocalityGeocoder) GeocodeLocality(_ context.Context, area string) (float64, float64, bool, error) {
	normalized := strings.ToLower(strings.TrimSpace(area))
	if normalized == "" {
		return 0, 0, false, nil
	}
	if coords, ok := localityCentroids[normalized]; ok {
		return coords[0], coords[1], true, nil
	}
	for name, coords := range localityCentroids {
		if strings.Contains(normalized, name) {
			return coords[0], coords[1], true, nil
		}
	}
	return 0, 0, false, nil
}

func validLatLng(lat, lng float64) bool {
	return !math.IsNaN(lat) && !math.IsInf(lat, 0) &&
		!math.IsNaN(lng) && !math.IsInf(lng, 0) &&
		lat >= -90 && lat <= 90 &&
		lng >= -180 && lng <= 180
}

// ValidatePublicCoordinates enforces that only premises-based services may
// store or publish coordinates, and validates latitude/longitude bounds.
func ValidatePublicCoordinates(deliveryMode string, lat, lng *float64) (*float64, *float64, error) {
	if deliveryMode != "premises" {
		return nil, nil, nil
	}
	if lat == nil && lng == nil {
		return nil, nil, nil
	}
	if lat == nil || lng == nil || !validLatLng(*lat, *lng) {
		return nil, nil, ErrInvalid
	}
	latCopy, lngCopy := *lat, *lng
	return &latCopy, &lngCopy, nil
}

// ValidateSearchRadius validates optional coordinate + radius discovery filters.
func ValidateSearchRadius(lat, lng *float64, radiusKM float64) error {
	if lat == nil && lng == nil && radiusKM == 0 {
		return nil
	}
	if lat == nil || lng == nil || !validLatLng(*lat, *lng) {
		return ErrInvalid
	}
	if math.IsNaN(radiusKM) || math.IsInf(radiusKM, 0) || radiusKM <= 0 || radiusKM > MaxSearchRadiusKM {
		return ErrInvalid
	}
	return nil
}

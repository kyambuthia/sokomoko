package domain

import (
	"errors"
	"time"

	"github.com/kyambuthia/sokomoko/internal/money"
)

var (
	ErrServiceNotFound = errors.New("service listing not found")
	ErrServiceConflict = errors.New("service listing changed; reload before editing")
)

type ServiceListing struct {
	ID              int                 `json:"id"`
	PartnerID       int                 `json:"partner_id"`
	PartnerName     string              `json:"partner_name"`
	PartnerSlug     string              `json:"partner_slug"`
	Slug            string              `json:"slug"`
	Title           string              `json:"title"`
	Description     string              `json:"description"`
	Category        string              `json:"category"`
	DurationMinutes int                 `json:"duration_minutes"`
	PriceMode       string              `json:"price_mode"`
	PriceCents      money.Cents         `json:"price_cents"`
	Currency        string              `json:"currency"`
	PublicArea      string              `json:"public_area"`
	DeliveryMode    string              `json:"delivery_mode"`
	State           string              `json:"state"`
	Version         int                 `json:"version"`
	CreatedAt       time.Time           `json:"created_at"`
	UpdatedAt       time.Time           `json:"updated_at"`
	Timezone        string              `json:"timezone"`
	OpenDays        int                 `json:"open_days"`
	PublicLat       *float64            `json:"public_lat,omitempty"`
	PublicLng       *float64            `json:"public_lng,omitempty"`
	DistanceKM      *float64            `json:"distance_km,omitempty"`
	WeeklyHours     []ServiceWeeklyHour `json:"weekly_hours,omitempty"`
}

// ServiceWeeklyHour uses Sunday=0 through Saturday=6 and local 24-hour times.
type ServiceWeeklyHour struct {
	Weekday  int    `json:"weekday"`
	OpensAt  string `json:"opens_at"`
	ClosesAt string `json:"closes_at"`
}

type ServiceFilter struct {
	Query, Category, Area, DeliveryMode string
	PartnerID, Limit, Offset            int
	HasHours                            bool
	Lat, Lng                            *float64
	RadiusKM                            float64
}

type ServiceBooking struct {
	ID               int64       `json:"id"`
	ServiceListingID int         `json:"service_listing_id"`
	ResourceID       int64       `json:"resource_id"`
	CustomerID       int         `json:"customer_id"`
	ServiceTitle     string      `json:"service_title"`
	ServiceArea      string      `json:"service_area"`
	DeliveryMode     string      `json:"delivery_mode"`
	DurationMinutes  int         `json:"duration_minutes"`
	PriceMode        string      `json:"price_mode"`
	PriceCents       money.Cents `json:"price_cents"`
	Currency         string      `json:"currency"`
	Timezone         string      `json:"timezone"`
	StartsAt         time.Time   `json:"starts_at"`
	EndsAt           time.Time   `json:"ends_at"`
	Status           string      `json:"status"`
	ResponseDeadline time.Time   `json:"response_deadline"`
	Version          int64       `json:"version"`
	CreatedAt        time.Time   `json:"created_at"`
	UpdatedAt        time.Time   `json:"updated_at"`
	RequestHash      []byte      `json:"-"`
}

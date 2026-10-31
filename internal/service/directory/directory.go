// Package directory manages public service discovery and seller-owned offerings.
// It does not reserve appointments, accept payments or store clinical records.
package directory

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/money"
	"github.com/kyambuthia/sokomoko/internal/slug"
)

var (
	ErrForbidden = errors.New("partner role required")
	ErrInvalid   = errors.New("invalid service listing")
)

type Actor struct {
	ID   int
	Role string
}
type Input struct {
	Title           string                 `json:"title"`
	Description     string                 `json:"description"`
	Category        string                 `json:"category"`
	DurationMinutes int                    `json:"duration_minutes"`
	PriceMode       string                 `json:"price_mode"`
	Price           string                 `json:"price"`
	Currency        string                 `json:"currency"`
	PublicArea      string                 `json:"public_area"`
	DeliveryMode    string                 `json:"delivery_mode"`
	State           string                 `json:"state"`
	Version         int                    `json:"version"`
	Timezone        string                 `json:"timezone"`
	WeeklyHours     []db.ServiceWeeklyHour `json:"weekly_hours"`
}
type store interface {
	BrowseServices(context.Context, db.ServiceFilter) ([]db.ServiceListing, int, error)
	PublicService(context.Context, string) (*db.ServiceListing, error)
	OwnService(context.Context, int, int) (*db.ServiceListing, error)
	OwnServicesByState(context.Context, int, string, int, int) ([]db.ServiceListing, int, error)
	SaveService(context.Context, int, db.ServiceListing) (*db.ServiceListing, error)
}
type Service struct{ store store }

func New(s store) *Service     { return &Service{store: s} }
func validActor(a *Actor) bool { return a != nil && a.ID > 0 && a.Role == db.RolePartner }
func text(s string, min, max int, multiline bool) bool {
	if !utf8.ValidString(s) || utf8.RuneCountInString(s) < min || utf8.RuneCountInString(s) > max {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) && !(multiline && (r == '\n' || r == '\t')) {
			return false
		}
	}
	return true
}
func oneOf(s string, values ...string) bool {
	for _, v := range values {
		if s == v {
			return true
		}
	}
	return false
}
func pageBounds(page, limit int) bool { return page >= 1 && page <= 1000 && limit >= 1 && limit <= 50 }
func (s *Service) Browse(ctx context.Context, f db.ServiceFilter, page int) ([]db.ServiceListing, int, error) {
	f.Query = strings.TrimSpace(f.Query)
	f.Area = strings.TrimSpace(f.Area)
	f.Category = strings.TrimSpace(f.Category)
	f.DeliveryMode = strings.TrimSpace(f.DeliveryMode)
	if f.PartnerID < 0 || f.PartnerID > 2147483647 || (f.DeliveryMode != "" && !oneOf(f.DeliveryMode, "premises", "mobile", "remote")) || !pageBounds(page, f.Limit) || !text(f.Query, 0, 120, false) || !text(f.Area, 0, 120, false) || (f.Category != "" && !oneOf(f.Category, "hair", "beauty", "dental", "wellness", "home", "other")) {
		return nil, 0, ErrInvalid
	}
	f.Offset = (page - 1) * f.Limit
	return s.store.BrowseServices(ctx, f)
}

const maxServiceSlugLength = 160

func (s *Service) Public(ctx context.Context, key string) (*db.ServiceListing, error) {
	if !slug.ValidWithMaxLength(key, maxServiceSlugLength) {
		return nil, db.ErrServiceNotFound
	}
	return s.store.PublicService(ctx, key)
}
func (s *Service) Own(ctx context.Context, a *Actor, page, limit int) ([]db.ServiceListing, int, error) {
	return s.OwnByState(ctx, a, "", page, limit)
}

func (s *Service) OwnByState(ctx context.Context, a *Actor, state string, page, limit int) ([]db.ServiceListing, int, error) {
	if !validActor(a) {
		return nil, 0, ErrForbidden
	}
	if !pageBounds(page, limit) || (state != "" && !oneOf(state, "draft", "published", "archived")) {
		return nil, 0, ErrInvalid
	}
	return s.store.OwnServicesByState(ctx, a.ID, state, limit, (page-1)*limit)
}
func (s *Service) Edit(ctx context.Context, a *Actor, id int) (*db.ServiceListing, error) {
	if !validActor(a) {
		return nil, ErrForbidden
	}
	if id <= 0 {
		return nil, db.ErrServiceNotFound
	}
	return s.store.OwnService(ctx, a.ID, id)
}

func (s *Service) Save(ctx context.Context, a *Actor, id int, in Input) (*db.ServiceListing, error) {
	if !validActor(a) {
		return nil, ErrForbidden
	}
	// Older API clients may not send the newly added schedule fields on update.
	// Preserve those values when omitted; an explicit empty array clears hours.
	if id > 0 && (in.Timezone == "" || in.WeeklyHours == nil) {
		current, err := s.store.OwnService(ctx, a.ID, id)
		if err != nil {
			return nil, err
		}
		if in.Timezone == "" {
			in.Timezone = current.Timezone
		}
		if in.WeeklyHours == nil {
			in.WeeklyHours = current.WeeklyHours
		}
	}
	if in.WeeklyHours == nil {
		in.WeeklyHours = []db.ServiceWeeklyHour{}
	}
	in.Title = strings.TrimSpace(in.Title)
	// URL-encoded browser forms submit textarea line breaks as CRLF.
	// Store canonical LF before checking length and control characters.
	in.Description = strings.TrimSpace(strings.ReplaceAll(in.Description, "\r\n", "\n"))
	in.PublicArea = strings.TrimSpace(in.PublicArea)
	in.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
	in.Timezone = strings.TrimSpace(in.Timezone)
	if in.Timezone == "" {
		in.Timezone = "Africa/Nairobi"
	}
	for i := range in.WeeklyHours {
		in.WeeklyHours[i].OpensAt = strings.TrimSpace(in.WeeklyHours[i].OpensAt)
		in.WeeklyHours[i].ClosesAt = strings.TrimSpace(in.WeeklyHours[i].ClosesAt)
	}
	sort.Slice(in.WeeklyHours, func(i, j int) bool { return in.WeeklyHours[i].Weekday < in.WeeklyHours[j].Weekday })
	price, err := money.Parse(strings.TrimSpace(in.Price))
	if in.PriceMode == "quote" && strings.TrimSpace(in.Price) == "" {
		price = 0
		err = nil
	}
	if validationErr := validateInput(id, in, price, err); validationErr != nil {
		return nil, validationErr
	}
	v := db.ServiceListing{ID: id, Title: in.Title, Description: in.Description, Category: in.Category, DurationMinutes: in.DurationMinutes, PriceMode: in.PriceMode, PriceCents: price, Currency: in.Currency, PublicArea: in.PublicArea, DeliveryMode: in.DeliveryMode, State: in.State, Version: in.Version, Timezone: in.Timezone, WeeklyHours: in.WeeklyHours}
	if id == 0 {
		var suffix [8]byte
		if _, err := rand.Read(suffix[:]); err != nil {
			return nil, err
		}
		base := slug.Make(in.Title)
		if len(base) > 120 {
			base = strings.TrimRight(base[:120], "-")
		}
		if base == "" {
			base = "service"
		}
		v.Slug = base + "-" + hex.EncodeToString(suffix[:])
	}
	return s.store.SaveService(ctx, a.ID, v)
}

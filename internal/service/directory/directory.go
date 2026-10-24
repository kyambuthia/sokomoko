// Package directory manages public service discovery and seller-owned offerings.
// It does not reserve appointments, accept payments or store clinical records.
package directory

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
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
	Title           string `json:"title"`
	Description     string `json:"description"`
	Category        string `json:"category"`
	DurationMinutes int    `json:"duration_minutes"`
	PriceMode       string `json:"price_mode"`
	Price           string `json:"price"`
	Currency        string `json:"currency"`
	PublicArea      string `json:"public_area"`
	DeliveryMode    string `json:"delivery_mode"`
	State           string `json:"state"`
	Version         int    `json:"version"`
}
type store interface {
	BrowseServices(context.Context, db.ServiceFilter) ([]db.ServiceListing, int, error)
	PublicService(context.Context, string) (*db.ServiceListing, error)
	OwnService(context.Context, int, int) (*db.ServiceListing, error)
	OwnServices(context.Context, int, int, int) ([]db.ServiceListing, int, error)
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
	if !pageBounds(page, f.Limit) || !text(f.Query, 0, 120, false) || !text(f.Area, 0, 120, false) || (f.Category != "" && !oneOf(f.Category, "hair", "beauty", "dental", "wellness", "home", "other")) {
		return nil, 0, ErrInvalid
	}
	f.Offset = (page - 1) * f.Limit
	return s.store.BrowseServices(ctx, f)
}
func (s *Service) Public(ctx context.Context, key string) (*db.ServiceListing, error) {
	if len(key) > 160 || !slug.Valid(key) {
		return nil, db.ErrServiceNotFound
	}
	return s.store.PublicService(ctx, key)
}
func (s *Service) Own(ctx context.Context, a *Actor, page, limit int) ([]db.ServiceListing, int, error) {
	if !validActor(a) {
		return nil, 0, ErrForbidden
	}
	if !pageBounds(page, limit) {
		return nil, 0, ErrInvalid
	}
	return s.store.OwnServices(ctx, a.ID, limit, (page-1)*limit)
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
	in.Title = strings.TrimSpace(in.Title)
	in.Description = strings.TrimSpace(in.Description)
	in.PublicArea = strings.TrimSpace(in.PublicArea)
	in.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
	price, err := money.Parse(strings.TrimSpace(in.Price))
	if in.PriceMode == "quote" && strings.TrimSpace(in.Price) == "" {
		price = 0
		err = nil
	}
	if id < 0 || (id > 0 && in.Version < 1) || (id == 0 && in.Version != 0) || err != nil || price < 0 || price > 100000000 || !text(in.Title, 3, 120, false) || !text(in.Description, 10, 3000, true) || !text(in.PublicArea, 2, 120, false) || !oneOf(in.Category, "hair", "beauty", "dental", "wellness", "home", "other") || in.DurationMinutes < 5 || in.DurationMinutes > 480 || !oneOf(in.PriceMode, "fixed", "from", "quote") || (in.PriceMode == "quote" && price != 0) || !oneOf(in.Currency, "KES", "USD") || !oneOf(in.DeliveryMode, "premises", "mobile", "remote") || !oneOf(in.State, "draft", "published", "archived") {
		return nil, ErrInvalid
	}
	v := db.ServiceListing{ID: id, Title: in.Title, Description: in.Description, Category: in.Category, DurationMinutes: in.DurationMinutes, PriceMode: in.PriceMode, PriceCents: price, Currency: in.Currency, PublicArea: in.PublicArea, DeliveryMode: in.DeliveryMode, State: in.State, Version: in.Version}
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

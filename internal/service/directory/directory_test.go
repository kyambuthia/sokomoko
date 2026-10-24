package directory

import (
	"errors"
	"github.com/kyambuthia/sokomoko/internal/db"
	"strings"
	"testing"
)

func validInput() Input {
	return Input{Title: "Demo haircut", Description: "Sample service description", Category: "hair", DurationMinutes: 30, PriceMode: "fixed", Price: "1000.00", Currency: "KES", PublicArea: "Westlands", DeliveryMode: "premises", State: "draft"}
}
func TestDirectoryRejectsUnauthorizedActorsBeforeStore(t *testing.T) {
	s := New(nil)
	for _, a := range []*Actor{nil, {ID: 1, Role: db.RoleUser}, {ID: 1, Role: db.RoleAdmin}, {ID: 1, Role: db.RoleCourier}, {ID: 0, Role: db.RolePartner}} {
		if _, err := s.Save(t.Context(), a, 0, validInput()); !errors.Is(err, ErrForbidden) {
			t.Fatal(err)
		}
		if _, err := s.Edit(t.Context(), a, 1); !errors.Is(err, ErrForbidden) {
			t.Fatal(err)
		}
		if _, _, err := s.Own(t.Context(), a, 1, 20); !errors.Is(err, ErrForbidden) {
			t.Fatal(err)
		}
	}
}
func TestDirectoryRejectsInvalidListingBeforeStore(t *testing.T) {
	s := New(nil)
	a := &Actor{ID: 1, Role: db.RolePartner}
	cases := []func(*Input){func(i *Input) { i.Title = "x" }, func(i *Input) { i.Title = "bad\x00title" }, func(i *Input) { i.Description = strings.Repeat("a", 3001) }, func(i *Input) { i.Description = "bad\xffdescription" }, func(i *Input) { i.DurationMinutes = 0 }, func(i *Input) { i.Price = "-1" }, func(i *Input) { i.Price = "1.001" }, func(i *Input) { i.PriceMode = "quote" }, func(i *Input) { i.Currency = "FAKE" }, func(i *Input) { i.Category = "unknown" }, func(i *Input) { i.State = "approved" }, func(i *Input) { i.PublicArea = "x" }, func(i *Input) { i.DeliveryMode = "unknown" }, func(i *Input) { i.Version = 1 }}
	for n, mutate := range cases {
		i := validInput()
		mutate(&i)
		if _, err := s.Save(t.Context(), a, 0, i); !errors.Is(err, ErrInvalid) {
			t.Fatalf("case %d: %v", n, err)
		}
	}
}
func TestDirectoryBoundsUntrustedQueries(t *testing.T) {
	s := New(nil)
	for _, f := range []db.ServiceFilter{{Limit: 0}, {Limit: 51}, {Limit: 20, Query: strings.Repeat("a", 121)}, {Limit: 20, Area: "bad\x00area"}, {Limit: 20, Category: "unknown"}, {Limit: 20, DeliveryMode: "unknown"}} {
		if _, _, err := s.Browse(t.Context(), f, 1); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
	}
	if _, _, err := s.Browse(t.Context(), db.ServiceFilter{Limit: 20}, 1001); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
}

func TestDirectoryReturnsAllFieldErrorsWithoutTouchingStore(t *testing.T) {
	in := validInput()
	in.Title = "x"
	in.DurationMinutes = 481
	in.Price = "not a price"
	in.Currency = "EUR"
	in.PublicArea = ""
	_, err := New(nil).Save(t.Context(), &Actor{ID: 1, Role: db.RolePartner}, 0, in)
	var validation *ValidationError
	if !errors.Is(err, ErrInvalid) || !errors.As(err, &validation) {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := []string{"title", "duration_minutes", "price", "public_area", "currency"}
	if len(validation.Fields) != len(expected) {
		t.Fatalf("fields=%+v", validation.Fields)
	}
	for i, field := range validation.Fields {
		if field.Field != expected[i] || field.Message == "" || strings.Contains(field.Message, "not a price") {
			t.Fatalf("field=%+v", field)
		}
	}
}

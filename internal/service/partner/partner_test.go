package partner

import (
	"errors"
	"testing"
)

func TestSaveStoreSettings_Validation(t *testing.T) {
	svc := newWithStore(nil)

	cases := []struct {
		input StoreSettingsInput
		want  error
	}{
		{StoreSettingsInput{}, ErrMissingStoreFields},
		{StoreSettingsInput{StoreName: "S", StoreSlug: "bad slug", ContactEmail: "a@b.co"}, ErrInvalidStoreSlug},
		{StoreSettingsInput{StoreName: "S", StoreSlug: "good", ContactEmail: "not-an-email"}, ErrInvalidContactEmail},
	}
	for _, tc := range cases {
		if _, err := svc.SaveStoreSettings(tc.input); !errors.Is(err, tc.want) {
			t.Errorf("SaveStoreSettings(%+v) = %v, want %v", tc.input, err, tc.want)
		}
	}
}

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"  Quiet Paper / Desk Lamp  ": "quiet-paper-desk-lamp",
		"Café Crème 2000":             "caf-cr-me-2000",
		"---":                         "",
		"Already-slugged":             "already-slugged",
	}
	for in, want := range cases {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestUpdateOrder_RequiresStaff(t *testing.T) {
	svc := newWithStore(nil)
	if err := svc.UpdateOrder(&Actor{ID: 1, Role: "user"}, UpdateOrderInput{OrderID: "1"}); !errors.Is(err, ErrForbiddenOrderUpdate) {
		t.Fatalf("err = %v", err)
	}
	if err := svc.UpdateOrder(&Actor{ID: 1, Role: "staff"}, UpdateOrderInput{OrderID: "x"}); !errors.Is(err, ErrInvalidOrderID) {
		t.Fatalf("err = %v", err)
	}
}

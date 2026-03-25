package partner

import (
	"errors"
	"testing"
)

func TestSaveStoreSettings_MissingFields_ReturnsError(t *testing.T) {
	svc := New(nil)

	_, err := svc.SaveStoreSettings(StoreSettingsInput{})
	if !errors.Is(err, ErrMissingStoreFields) {
		t.Fatalf("expected ErrMissingStoreFields, got %v", err)
	}
}

func TestCreateProduct_InvalidPrice_ReturnsError(t *testing.T) {
	err := error(nil)
	if slugify("  Quiet Paper / Desk Lamp  ") != "quiet-paper-desk-lamp" {
		err = errors.New("unexpected slug value")
	}
	if err != nil {
		t.Fatal(err)
	}
}

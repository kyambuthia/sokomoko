package app

import (
	"testing"

	"github.com/kyambuthia/sokomoko/internal/auth"
	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/db/dbtest"
	"github.com/kyambuthia/sokomoko/internal/ui"
)

func TestCompose_WiresFeatureServices(t *testing.T) {
	store, err := db.OpenStore(dbtest.DSN(t))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	if err := store.ApplySchema(); err != nil {
		t.Fatalf("apply schema: %v", err)
	}

	templates, err := ui.ParseTemplates()
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}

	application := Compose(ComposeOptions{
		Store:     store,
		Templates: templates,
		StaticFS:  ui.StaticFS,
		Auth: auth.Config{
			Environment:         "test",
			AdminSetupToken:     "setup-token",
			SessionCookieDomain: "localhost",
		},
	})

	if application.Readiness == nil {
		t.Fatal("expected readiness checker")
	}
	if application.Catalog == nil {
		t.Fatal("expected catalog service")
	}
	if application.Account == nil {
		t.Fatal("expected account service")
	}
	if application.Commerce == nil {
		t.Fatal("expected commerce service")
	}
	if application.Admin == nil {
		t.Fatal("expected admin service")
	}
	if application.Partner == nil {
		t.Fatal("expected partner service")
	}
	if application.Auth == nil {
		t.Fatal("expected auth service")
	}
	if application.Templates == nil {
		t.Fatal("expected templates")
	}
}

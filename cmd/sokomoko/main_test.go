package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/kyambuthia/sokomoko/internal/config"
	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/db/dbtest"
)

func TestParseCommand(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    commandConfig
		wantErr bool
	}{
		{
			name: "default serve",
			want: commandConfig{Name: commandServe},
		},
		{
			name: "serve with seed",
			args: []string{"serve", "--seed"},
			want: commandConfig{Name: commandServe, SeedOnServe: true},
		},
		{
			name: "migrate",
			args: []string{"migrate"},
			want: commandConfig{Name: commandMigrate},
		},
		{
			name: "seed",
			args: []string{"seed"},
			want: commandConfig{Name: commandSeed},
		},
		{
			name: "healthcheck",
			args: []string{"healthcheck"},
			want: commandConfig{Name: commandHealthcheck},
		},
		{
			name: "help",
			args: []string{"help"},
			want: commandConfig{ShowHelpOnly: true},
		},
		{
			name:    "unknown command",
			args:    []string{"unknown"},
			wantErr: true,
		},
		{
			name:    "unsupported serve flag",
			args:    []string{"serve", "--bad"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseCommand(tt.args)
			if tt.wantErr {
				if !errors.Is(err, ErrUsage) {
					t.Fatalf("expected ErrUsage, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseCommand returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("parseCommand(%v) = %#v, want %#v", tt.args, got, tt.want)
			}
		})
	}
}

func TestRunMigrate_AppliesSchema(t *testing.T) {
	cfg := config.Config{DatabaseURL: dbtest.DSN(t)}

	if err := runMigrate(cfg); err != nil {
		t.Fatalf("runMigrate failed: %v", err)
	}
	assertSchemaApplied(t, cfg.DatabaseURL)
}

func TestRunSeed_BootstrapsCatalog(t *testing.T) {
	cfg := config.Config{DatabaseURL: dbtest.DSN(t)}

	if err := runSeed(cfg); err != nil {
		t.Fatalf("runSeed failed: %v", err)
	}
	if countProducts(t, cfg.DatabaseURL) == 0 {
		t.Fatal("expected seed command to create catalog products")
	}
}

func TestRunMigrate_ErrorIncludesCommandContext(t *testing.T) {
	cfg := config.Config{DatabaseURL: "postgres://nobody@127.0.0.1:1/missing?sslmode=disable&connect_timeout=1"}

	err := runMigrate(cfg)
	if err == nil {
		t.Fatal("expected migrate to fail for an unreachable database")
	}
	if !strings.Contains(err.Error(), "migrate: open store:") {
		t.Fatalf("expected command-scoped error, got %v", err)
	}
}

func TestRunServe_RejectsInvalidProductionConfig(t *testing.T) {
	err := runServe(config.Config{Environment: "production", Port: "6969", LogFormat: "json"}, false)
	if err == nil || !strings.Contains(err.Error(), "validate config") {
		t.Fatalf("expected config validation error, got %v", err)
	}
}

func openExistingStore(t *testing.T, dsn string) *db.Store {
	t.Helper()

	store, err := db.OpenStore(dsn)
	if err != nil {
		t.Fatalf("open existing store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestBuildBlobStore(t *testing.T) {
	memStore, err := buildBlobStore(config.Config{MediaBackend: "memory"})
	if err != nil {
		t.Fatalf("memory blob store: %v", err)
	}
	if err := memStore.Put(t.Context(), "products/1/video", "video/mp4", []byte("mp4")); err != nil {
		t.Fatalf("put to memory store: %v", err)
	}

	dir := t.TempDir()
	diskStore, err := buildBlobStore(config.Config{MediaBackend: "disk", MediaDiskDir: dir})
	if err != nil {
		t.Fatalf("disk blob store: %v", err)
	}
	if err := diskStore.Put(t.Context(), "products/1/images/1", "image/png", []byte("png")); err != nil {
		t.Fatalf("put to disk store: %v", err)
	}
	data, contentType, err := diskStore.Get(t.Context(), "products/1/images/1")
	if err != nil {
		t.Fatalf("get from disk store: %v", err)
	}
	if string(data) != "png" || contentType != "image/png" {
		t.Fatalf("got (%q, %q), want (png, image/png)", string(data), contentType)
	}

	if _, err := buildBlobStore(config.Config{MediaBackend: "unknown"}); err == nil {
		t.Fatal("expected error for unsupported media backend")
	}
}

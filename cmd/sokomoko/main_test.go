package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/kyambuthia/sokomoko/internal/config"
	"github.com/kyambuthia/sokomoko/internal/db"
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
	cfg := config.Config{DBPath: tempDBPath(t, "migrate")}

	if err := runMigrate(cfg); err != nil {
		t.Fatalf("runMigrate failed: %v", err)
	}

	store := openExistingStore(t, cfg.DBPath)

	var count int
	if err := store.DB.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='users'").Scan(&count); err != nil {
		t.Fatalf("query schema: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected users table to exist, got count=%d", count)
	}
}

func TestRunSeed_BootstrapsCatalog(t *testing.T) {
	cfg := config.Config{DBPath: tempDBPath(t, "seed")}

	if err := runSeed(cfg); err != nil {
		t.Fatalf("runSeed failed: %v", err)
	}

	store := openExistingStore(t, cfg.DBPath)

	productCount, err := store.CountProducts()
	if err != nil {
		t.Fatalf("count products: %v", err)
	}
	if productCount == 0 {
		t.Fatal("expected seed command to create catalog products")
	}
}

func tempDBPath(t *testing.T, prefix string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), prefix+".db")
	return path
}

func openExistingStore(t *testing.T, path string) *db.Store {
	t.Helper()

	store, err := db.OpenStore(path)
	if err != nil {
		t.Fatalf("open existing store: %v", err)
	}

	t.Cleanup(func() {
		_ = store.Close()
		_ = os.Remove(path)
	})
	return store
}

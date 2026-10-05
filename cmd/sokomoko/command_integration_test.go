package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/kyambuthia/sokomoko/internal/config"
	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/db/dbtest"
)

func TestCLI_MigrateCommand_AppliesSchemaAndIsIdempotent(t *testing.T) {
	binaryPath := buildCLIBinary(t)
	dbPath := dbtest.DSN(t)

	runCLICommand(t, binaryPath, dbPath, "", "migrate")
	assertSchemaApplied(t, dbPath)

	runCLICommand(t, binaryPath, dbPath, "", "migrate")
	assertSchemaApplied(t, dbPath)
}

func TestCLI_SeedCommand_BootstrapsCatalogAndIsIdempotent(t *testing.T) {
	binaryPath := buildCLIBinary(t)
	dbPath := dbtest.DSN(t)

	runCLICommand(t, binaryPath, dbPath, "", "seed")
	firstCount := countProducts(t, dbPath)
	if firstCount == 0 {
		t.Fatal("expected seed command to create catalog products")
	}

	runCLICommand(t, binaryPath, dbPath, "", "seed")
	secondCount := countProducts(t, dbPath)
	if secondCount != firstCount {
		t.Fatalf("expected seed command to be idempotent, got first=%d second=%d", firstCount, secondCount)
	}
}

func TestRunServe_SeedOption_PreparesStoreBeforeHTTPServe(t *testing.T) {
	dbPath := dbtest.DSN(t)

	originalRunner := runHTTPServerFunc
	t.Cleanup(func() {
		runHTTPServerFunc = originalRunner
	})

	runHTTPServerFunc = func(_ *httpServer, _ *db.Store) error {
		if count := countProducts(t, dbPath); count == 0 {
			t.Fatal("expected runServe with seed to create catalog products before serving")
		}
		return nil
	}

	err := runServe(config.Config{
		DatabaseURL:     dbPath,
		Port:            "6969",
		LogFormat:       "text",
		AllowedHostsRaw: "localhost,127.0.0.1,admin.localhost,partner.localhost",
	}, true)
	if err != nil {
		t.Fatalf("runServe failed: %v", err)
	}
}

func buildCLIBinary(t *testing.T) string {
	t.Helper()

	binaryPath := filepath.Join(t.TempDir(), "sokomoko-test-bin")
	cmd := exec.Command("go", "build", "-o", binaryPath, ".")
	cmd.Dir = "."

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build CLI binary failed: %v\noutput:\n%s", err, string(output))
	}
	return binaryPath
}

func runCLICommand(t *testing.T, binaryPath, dbPath, port string, args ...string) string {
	t.Helper()

	cmd := exec.Command(binaryPath, args...)
	cmd.Env = cliEnv(dbPath, port)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run CLI command %v failed: %v\noutput:\n%s", args, err, string(output))
	}
	return string(output)
}

func cliEnv(dbPath, port string) []string {
	env := append([]string{}, os.Environ()...)
	env = append(env, "DATABASE_URL="+dbPath)
	if port != "" {
		env = append(env, "PORT="+port)
	}
	return env
}

func assertSchemaApplied(t *testing.T, dsn string) {
	t.Helper()

	store := openExistingStore(t, dsn)
	version, err := store.SchemaVersion()
	if err != nil {
		t.Fatalf("schema version: %v", err)
	}
	if version != db.LatestSchemaVersion() {
		t.Fatalf("schema version = %d, want %d", version, db.LatestSchemaVersion())
	}
}

func countProducts(t *testing.T, dbPath string) int {
	t.Helper()

	store := openExistingStore(t, dbPath)
	count, err := store.CountProducts()
	if err != nil {
		t.Fatalf("count products: %v", err)
	}
	return count
}

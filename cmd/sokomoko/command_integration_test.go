package main

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCLI_MigrateCommand_AppliesSchemaAndIsIdempotent(t *testing.T) {
	binaryPath := buildCLIBinary(t)
	dbPath := tempDBPath(t, "cli-migrate")

	runCLICommand(t, binaryPath, dbPath, "", "migrate")
	assertSchemaApplied(t, dbPath)

	runCLICommand(t, binaryPath, dbPath, "", "migrate")
	assertSchemaApplied(t, dbPath)
}

func TestCLI_SeedCommand_BootstrapsCatalogAndIsIdempotent(t *testing.T) {
	binaryPath := buildCLIBinary(t)
	dbPath := tempDBPath(t, "cli-seed")

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

func TestCLI_ServeSeedCommand_StartsServerAndSeedsCatalog(t *testing.T) {
	binaryPath := buildCLIBinary(t)
	dbPath := tempDBPath(t, "cli-serve-seed")
	port := freePort(t)

	cmd, logs := startCLICommand(t, binaryPath, dbPath, port, "serve", "--seed")
	waitForHealth(t, port)

	if count := countProducts(t, dbPath); count == 0 {
		t.Fatal("expected serve --seed to create catalog products")
	}

	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatalf("interrupt server: %v", err)
	}

	err := cmd.Wait()
	if err != nil && !strings.Contains(logs.String(), "server stopped gracefully") {
		t.Fatalf("serve --seed exited unexpectedly: %v\noutput:\n%s", err, logs.String())
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

func startCLICommand(t *testing.T, binaryPath, dbPath, port string, args ...string) (*exec.Cmd, *bytes.Buffer) {
	t.Helper()

	cmd := exec.Command(binaryPath, args...)
	cmd.Env = cliEnv(dbPath, port)

	var logs bytes.Buffer
	cmd.Stdout = &logs
	cmd.Stderr = &logs

	if err := cmd.Start(); err != nil {
		t.Fatalf("start CLI command %v failed: %v", args, err)
	}

	t.Cleanup(func() {
		if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
			return
		}
		_ = cmd.Process.Signal(os.Interrupt)
		_ = cmd.Wait()
	})

	return cmd, &logs
}

func cliEnv(dbPath, port string) []string {
	env := append([]string{}, os.Environ()...)
	env = append(env, "DB_PATH="+dbPath)
	if port != "" {
		env = append(env, "PORT="+port)
	}
	return env
}

func waitForHealth(t *testing.T, port string) {
	t.Helper()

	client := &http.Client{Timeout: 500 * time.Millisecond}
	url := fmt.Sprintf("http://localhost:%s/healthz", port)
	deadline := time.Now().Add(10 * time.Second)

	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("health check did not succeed for %s", url)
}

func freePort(t *testing.T) string {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	defer ln.Close()

	return fmt.Sprintf("%d", ln.Addr().(*net.TCPAddr).Port)
}

func assertSchemaApplied(t *testing.T, dbPath string) {
	t.Helper()

	store := openExistingStore(t, dbPath)
	var count int
	if err := store.DB.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='users'").Scan(&count); err != nil {
		t.Fatalf("query schema: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected users table to exist, got count=%d", count)
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

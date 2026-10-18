package main

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestIntegration_AdminReactivatesDeactivatedUser(t *testing.T) {
	clearAllTables()
	suffix := time.Now().UnixNano() % 1_000_000_000
	createTestUser(t, fmt.Sprintf("boss_%d", suffix), fmt.Sprintf("boss_%d@example.com", suffix), "AdminPass12345", "admin")
	worker := fmt.Sprintf("worker_%d", suffix)
	workerID := createTestUser(t, worker, worker+"@example.com", "StaffPass12345", "staff")
	admin := loginAndGetSessionCookie(t, "admin.localhost", fmt.Sprintf("boss_%d", suffix), "AdminPass12345")

	resp, _ := makeRequest(http.MethodPost, "/team", url.Values{"user_id": {fmt.Sprint(workerID)}}, []*http.Cookie{admin}, "admin.localhost")
	assertFlashRedirect(t, resp, "/team", "deactivated")
	if resp, _ := makeRequest(http.MethodPost, "/login", url.Values{"username": {worker}, "password": {"StaffPass12345"}}, nil, "admin.localhost"); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("deactivated login status=%d", resp.StatusCode)
	}
	if _, body := makeRequest(http.MethodGet, "/team", nil, []*http.Cookie{admin}, "admin.localhost"); !strings.Contains(body, "Deactivated") || !strings.Contains(body, `value="reactivate"`) {
		t.Fatal("team page does not list the deactivated user")
	}

	resp, _ = makeRequest(http.MethodPost, "/team", url.Values{"action": {"reactivate"}, "user_id": {fmt.Sprint(workerID)}}, []*http.Cookie{admin}, "admin.localhost")
	assertFlashRedirect(t, resp, "/team", "reactivated")
	loginAndGetSessionCookie(t, "admin.localhost", worker, "StaffPass12345")

	// Reactivating an active or unknown user reports it.
	resp, body := makeRequest(http.MethodPost, "/team", url.Values{"action": {"reactivate"}, "user_id": {fmt.Sprint(workerID)}}, []*http.Cookie{admin}, "admin.localhost")
	if resp.StatusCode == http.StatusFound || !strings.Contains(body, "User does not exist") {
		t.Fatalf("reactivate active user status=%d", resp.StatusCode)
	}
	var audits int
	if err := testStore.DB.QueryRow("SELECT count(*) FROM audit_logs WHERE action = 'user.reactivate'").Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("reactivate audits = %d, %v", audits, err)
	}
}

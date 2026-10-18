package main

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestIntegration_SignedInPasswordChange(t *testing.T) {
	clearAllTables()
	suffix := time.Now().UnixNano() % 1_000_000_000
	name := fmt.Sprintf("changer_%d", suffix)
	createTestUser(t, name, name+"@example.com", "OldPassword123", "user")
	laptop := loginAndGetSessionCookie(t, "localhost", name, "OldPassword123")
	phone := loginAndGetSessionCookie(t, "localhost", name, "OldPassword123")

	if _, body := makeRequest(http.MethodGet, "/account", nil, []*http.Cookie{laptop}, "localhost"); !strings.Contains(body, `href="/account/password"`) {
		t.Fatal("account page has no password link")
	}
	if resp, _ := makeRequest(http.MethodGet, "/account/password", nil, nil, "localhost"); resp.StatusCode == http.StatusOK {
		t.Fatal("password page served without a session")
	}

	for _, tc := range []struct {
		form url.Values
		want string
	}{
		{url.Values{"current_password": {"WrongPassword1"}, "password": {"NewPassword456"}, "confirm_password": {"NewPassword456"}}, "Current password is incorrect"},
		{url.Values{"current_password": {"OldPassword123"}, "password": {"NewPassword456"}, "confirm_password": {"Different4567"}}, "do not match"},
		{url.Values{"current_password": {"OldPassword123"}, "password": {"short"}, "confirm_password": {"short"}}, "10 to 72"},
		{url.Values{"current_password": {"OldPassword123"}, "password": {"OldPassword123"}, "confirm_password": {"OldPassword123"}}, "not been using"},
	} {
		resp, body := makeRequest(http.MethodPost, "/account/password", tc.form, []*http.Cookie{laptop}, "localhost")
		if resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, tc.want) {
			t.Fatalf("form %v status=%d, want error %q", tc.form, resp.StatusCode, tc.want)
		}
	}

	resp, _ := makeRequest(http.MethodPost, "/account/password", url.Values{
		"current_password": {"OldPassword123"}, "password": {"NewPassword456"}, "confirm_password": {"NewPassword456"},
	}, []*http.Cookie{laptop}, "localhost")
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/account/password?changed=1" {
		t.Fatalf("change status=%d location=%q", resp.StatusCode, resp.Header.Get("Location"))
	}
	fresh := getSessionCookie(resp)
	if fresh == nil || fresh.Value == laptop.Value {
		t.Fatal("password change did not issue a new session")
	}
	if resp, body := makeRequest(http.MethodGet, "/account/password?changed=1", nil, []*http.Cookie{fresh}, "localhost"); resp.StatusCode != http.StatusOK || !strings.Contains(body, "Password changed") {
		t.Fatalf("new session status=%d", resp.StatusCode)
	}
	for label, old := range map[string]*http.Cookie{"laptop": laptop, "phone": phone} {
		if resp, _ := makeRequest(http.MethodGet, "/account", nil, []*http.Cookie{old}, "localhost"); resp.StatusCode == http.StatusOK {
			t.Fatalf("%s session survived the password change", label)
		}
	}
	if resp, _ := makeRequest(http.MethodPost, "/login", url.Values{"username": {name}, "password": {"OldPassword123"}}, nil, "localhost"); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("old password login status=%d", resp.StatusCode)
	}
	loginAndGetSessionCookie(t, "localhost", name, "NewPassword456")
}

func TestIntegration_WorkspacePasswordChange(t *testing.T) {
	clearAllTables()
	suffix := time.Now().UnixNano() % 1_000_000_000
	setupPartnerStore(t, suffix)
	staff := fmt.Sprintf("ops_%d", suffix)
	createTestUser(t, staff, staff+"@example.com", "StaffPass12345", "staff")
	createTestUser(t, fmt.Sprintf("root_%d", suffix), fmt.Sprintf("root_%d@example.com", suffix), "AdminPass12345", "admin")
	for _, host := range []string{"admin.localhost", "partner.localhost"} {
		password := "StaffPass12345"
		if host == "partner.localhost" {
			password = "StaffPass67890"
		}
		next := map[string]string{"admin.localhost": "StaffPass67890", "partner.localhost": "StaffPass13579"}[host]
		cookie := loginAndGetSessionCookie(t, host, staff, password)
		if _, body := makeRequest(http.MethodGet, "/password", nil, []*http.Cookie{cookie}, host); !strings.Contains(body, "Change password") {
			t.Fatalf("%s: password page missing", host)
		}
		resp, _ := makeRequest(http.MethodPost, "/password", url.Values{
			"current_password": {password}, "password": {next}, "confirm_password": {next},
		}, []*http.Cookie{cookie}, host)
		if resp.StatusCode != http.StatusSeeOther || getSessionCookie(resp) == nil {
			t.Fatalf("%s: change status=%d", host, resp.StatusCode)
		}
		loginAndGetSessionCookie(t, host, staff, next)
	}
}

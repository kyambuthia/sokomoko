package auth

import (
	"context"
	"errors"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/kyambuthia/sokomoko/internal/db"
)

func TestCSRFMiddleware_StaleSessionCookieIsTreatedAsAnonymous(t *testing.T) {
	clearAuthTables()
	svc := newTestService(Config{})

	reached := false
	handler := svc.CSRFMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		if _, err := r.Cookie(sessionCookieName); err == nil {
			t.Errorf("stale session cookie leaked to the handler")
		}
	}))

	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("username=a&password=b"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "revoked-session"})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !reached {
		t.Fatalf("request with stale cookie was blocked: status %d", rr.Code)
	}
	cleared := false
	for _, c := range rr.Result().Cookies() {
		if c.Name == sessionCookieName && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Fatalf("stale session cookie was not cleared")
	}
}

func TestCSRFMiddleware_ValidSessionRequiresToken(t *testing.T) {
	clearAuthTables()
	user := createTestUser("csrfuser", "csrf@example.com", "password123", db.RoleUser)
	_ = testStore.CreateSession(db.Session{ID: "live", UserID: user.ID, CSRFToken: "expected", ExpiresAt: time.Now().Add(time.Hour)})
	svc := newTestService(Config{})
	handler := svc.CSRFMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	for token, want := range map[string]int{"": http.StatusForbidden, "wrong": http.StatusForbidden, "expected": http.StatusOK} {
		req := httptest.NewRequest(http.MethodPost, "/cart/add", nil)
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "live"})
		if token != "" {
			req.Header.Set("X-CSRF-Token", token)
		}
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != want {
			t.Errorf("token %q: status %d, want %d", token, rr.Code, want)
		}
	}
}

func TestSafeNextPath(t *testing.T) {
	cases := map[string]string{
		"":                     "/",
		"/cart":                "/cart",
		"/checkout?checkout=x": "/checkout?checkout=x",
		"//evil.example":       "/",
		"https://evil.example": "/",
		"/\\evil.example":      "/",
		"javascript:alert(1)":  "/",
	}
	for in, want := range cases {
		if got := safeNextPath(in); got != want {
			t.Errorf("safeNextPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestClientIP_IgnoresForwardedForUnlessTrusted(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.5:1234"
	req.Header.Set("X-Forwarded-For", "203.0.113.9, 10.0.0.1")

	if got := ClientIP(req, false); got != "10.0.0.5" {
		t.Fatalf("untrusted ClientIP = %q, want remote addr", got)
	}
	if got := ClientIP(req, true); got != "10.0.0.1" {
		t.Fatalf("trusted ClientIP = %q, want the hop the proxy wrote", got)
	}
}

func TestClientIP_IgnoresClientSuppliedForwardedEntries(t *testing.T) {
	// An appending proxy keeps whatever the client sent and adds the real
	// address last. Rotating the spoofed entry must not change the key.
	for _, spoofed := range []string{"198.51.100.1", "198.51.100.2"} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "127.0.0.1:1234"
		req.Header.Add("X-Forwarded-For", spoofed)
		req.Header.Add("X-Forwarded-For", "203.0.113.9")
		if got := ClientIP(req, true); got != "203.0.113.9" {
			t.Fatalf("ClientIP with spoofed %s = %q", spoofed, got)
		}
	}
}

func TestIsLoopbackRequest_RejectsProxiedRequests(t *testing.T) {
	direct := httptest.NewRequest(http.MethodGet, "/setup", nil)
	direct.RemoteAddr = "127.0.0.1:5000"
	if !isLoopbackRequest(direct) {
		t.Fatal("direct loopback request rejected")
	}
	for _, header := range []string{"X-Forwarded-For", "X-Real-Ip", "Forwarded", "Via"} {
		proxied := httptest.NewRequest(http.MethodGet, "/setup", nil)
		proxied.RemoteAddr = "127.0.0.1:5000"
		proxied.Header.Set(header, "203.0.113.9")
		if isLoopbackRequest(proxied) {
			t.Fatalf("request relayed with %s treated as loopback", header)
		}
	}
	remote := httptest.NewRequest(http.MethodGet, "/setup", nil)
	remote.RemoteAddr = "203.0.113.9:5000"
	if isLoopbackRequest(remote) {
		t.Fatal("remote request treated as loopback")
	}
}

func TestWorkspaceLogin_NoAdminDoesNotRedirectLoop(t *testing.T) {
	clearAuthTables()
	tmpl := template.Must(template.New("x").Parse(`{{define "root_template"}}{{.Error}}{{end}}`))
	rr := httptest.NewRecorder()
	newTestService(Config{}).WorkspaceLogin(tmpl).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/login", nil))

	if rr.Code != http.StatusServiceUnavailable || rr.Header().Get("Location") != "" {
		t.Fatalf("status = %d location = %q, want 503 without redirect", rr.Code, rr.Header().Get("Location"))
	}
}

func TestLogin_RedirectsToSafeNextAndAcceptsEmail(t *testing.T) {
	clearAuthTables()
	createTestUser("shopper", "shopper@example.com", "password123", db.RoleUser)
	tmpl := template.Must(template.New("x").Parse(`{{define "root_template"}}{{.Error}}{{end}}`))

	form := url.Values{"username": {"Shopper@Example.com"}, "password": {"password123"}, "next": {"/cart"}}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	newTestService(Config{}).Login(tmpl).ServeHTTP(rr, req)

	if rr.Code != http.StatusFound || rr.Header().Get("Location") != "/cart" {
		t.Fatalf("status = %d location = %q", rr.Code, rr.Header().Get("Location"))
	}
}

func TestAuthMiddleware_JSONClientsGet401(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/cart/summary", nil)
	req.Header.Set("Accept", "application/json")
	rr := httptest.NewRecorder()
	newTestService(Config{}).AuthMiddleware(http.NotFoundHandler()).ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}

func TestHashPassword_RejectsOverlongPasswords(t *testing.T) {
	if _, err := hashPassword(strings.Repeat("a", 73)); err == nil {
		t.Fatalf("expected error for >72 byte password")
	}
	if isStrongEnoughPassword(strings.Repeat("a", 73)) {
		t.Fatalf("73 byte password accepted")
	}
}

func TestAdminSetup_CreatesAdminAndStaffOnce(t *testing.T) {
	clearAuthTables()
	tmpl := template.Must(template.New("x").Parse(`{{define "root_template"}}{{.Error}}{{range .StaffCredentials}}[{{.Username}}:{{.TempPassword}}]{{end}}{{end}}`))
	svc := newTestService(Config{AdminSetupToken: "setup-secret"})

	submit := func() *httptest.ResponseRecorder {
		form := url.Values{
			"setup_token":      {"setup-secret"},
			"admin_username":   {"rootadmin"},
			"admin_email":      {"root@example.com"},
			"admin_password":   {"RootPassword123"},
			"confirm_password": {"RootPassword123"},
			"staff_count":      {"2"},
		}
		req := httptest.NewRequest(http.MethodPost, "/setup", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()
		svc.AdminSetup(tmpl).ServeHTTP(rr, req)
		return rr
	}

	rr := submit()
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "[staff01:") || !strings.Contains(rr.Body.String(), "[staff02:") {
		t.Fatalf("first setup status=%d body=%q", rr.Code, rr.Body.String())
	}
	staff, _ := testStore.GetUserByUsername("staff01")
	if staff == nil || staff.Role != db.RoleStaff {
		t.Fatalf("staff01 = %+v", staff)
	}

	if again := submit(); again.Code != http.StatusFound || again.Header().Get("Location") != "/login" {
		t.Fatalf("second setup status=%d location=%q, want redirect to /login", again.Code, again.Header().Get("Location"))
	}
}

func TestDispatchPasswordResetEmail_RetriesTransientFailures(t *testing.T) {
	for _, tc := range []struct {
		name      string
		failures  []error
		wantCalls int
	}{
		{"succeeds after two transient failures", []error{errors.New("dial tcp: timeout"), errors.New("421 try later")}, 3},
		{"gives up after three attempts", []error{errors.New("a"), errors.New("b"), errors.New("c"), errors.New("d")}, 3},
		{"stops on a permanent rejection", []error{&textproto.Error{Code: 550, Msg: "no such user"}}, 1},
	} {
		calls := make(chan int, 10)
		n := 0
		svc := &Service{resetEmailBackoff: []time.Duration{time.Millisecond, time.Millisecond}, passwordResetEmailSender: func(context.Context, string, string) error {
			n++
			calls <- n
			if n <= len(tc.failures) {
				return tc.failures[n-1]
			}
			return nil
		}}
		svc.dispatchPasswordResetEmail("a@example.com", "https://shop.example/reset", 1)
		got := 0
		for got < tc.wantCalls {
			select {
			case got = <-calls:
			case <-time.After(2 * time.Second):
				t.Fatalf("%s: %d calls, want %d", tc.name, got, tc.wantCalls)
			}
		}
		select {
		case extra := <-calls:
			t.Fatalf("%s: unexpected call %d", tc.name, extra)
		case <-time.After(50 * time.Millisecond):
		}
	}
}

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
	"github.com/kyambuthia/sokomoko/internal/domain"
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
	user := createTestUser("csrfuser", "csrf@example.com", "password123", domain.RoleUser)
	_ = testStore.CreateSession(t.Context(), db.Session{ID: "live", UserID: user.ID, CSRFToken: "expected", ExpiresAt: time.Now().Add(time.Hour)})
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
	createTestUser("shopper", "shopper@example.com", "password123", domain.RoleUser)
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

func TestAdminSetup_CreatesAdminOnce(t *testing.T) {
	clearAuthTables()
	tmpl := template.Must(template.New("x").Parse(`{{define "root_template"}}{{.Error}}|{{.Message}}{{end}}`))
	svc := newTestService(Config{AdminSetupToken: "setup-secret"})

	submit := func() *httptest.ResponseRecorder {
		form := url.Values{
			"setup_token":      {"setup-secret"},
			"admin_username":   {"rootadmin"},
			"admin_email":      {"root@example.com"},
			"admin_password":   {"RootPassword123"},
			"confirm_password": {"RootPassword123"},
		}
		req := httptest.NewRequest(http.MethodPost, "/setup", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()
		svc.AdminSetup(tmpl).ServeHTTP(rr, req)
		return rr
	}

	rr := submit()
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "Root admin account has been created") {
		t.Fatalf("first setup status=%d body=%q", rr.Code, rr.Body.String())
	}
	adminUser, _ := testStore.GetUserByUsername(t.Context(), "rootadmin")
	if adminUser == nil || adminUser.Role != domain.RoleAdmin {
		t.Fatalf("rootadmin = %+v", adminUser)
	}
	if staff, _ := testStore.GetUserByUsername(t.Context(), "staff01"); staff != nil {
		t.Fatalf("unexpected staff created during first setup: %+v", staff)
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

func TestClientIP_TrailingCommaAndInvalidEntries(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.5:1234"
	req.Header.Set("X-Forwarded-For", "203.0.113.9, ")
	if got := ClientIP(req, true); got != "203.0.113.9" {
		t.Fatalf("trailing comma ClientIP = %q, want 203.0.113.9", got)
	}

	req.Header.Set("X-Forwarded-For", "203.0.113.9, not-an-ip")
	if got := ClientIP(req, true); got != "203.0.113.9" {
		t.Fatalf("trailing invalid entry ClientIP = %q, want 203.0.113.9", got)
	}

	req.Header.Set("X-Forwarded-For", "garbage, , also-bad")
	if got := ClientIP(req, true); got != "10.0.0.5" {
		t.Fatalf("all-invalid ClientIP = %q, want 10.0.0.5", got)
	}
}

func TestLogin_IPLockoutAcrossUsernames(t *testing.T) {
	clearAuthTables()
	svc := newTestService(Config{
		AuthAbuseMaxFailures: 2,
		AuthAbuseBackoffBase: time.Minute,
		AuthAbuseBackoffMax:  5 * time.Minute,
	})
	tmpl := template.Must(template.New("login.html").Parse(`{{define "root_template"}}{{.Error}}{{end}}`))
	handler := svc.Login(tmpl)

	for _, username := range []string{"nope1", "nope2"} {
		form := url.Values{"username": {username}, "password": {"wrong-password"}}
		req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.RemoteAddr = "198.51.100.10:1234"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("attempt for %s: status=%d, want 401", username, rr.Code)
		}
	}

	form := url.Values{"username": {"nope3"}, "password": {"wrong-password"}}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "198.51.100.10:1234"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("third attempt across new username: status=%d, want 429", rr.Code)
	}
}

func TestLoginAPI_IPLockoutAndSessionRevocation(t *testing.T) {
	clearAuthTables()
	user := createTestUser("apilogin", "apilogin@example.com", "correct-horse-battery", domain.RoleUser)
	_ = testStore.CreateSession(t.Context(), db.Session{
		ID:        "pre-login-session",
		UserID:    user.ID,
		CSRFToken: "old-csrf",
		ExpiresAt: time.Now().Add(time.Hour),
	})

	svc := newTestService(Config{
		AuthAbuseMaxFailures: 2,
		AuthAbuseBackoffBase: time.Minute,
		AuthAbuseBackoffMax:  5 * time.Minute,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.RemoteAddr = "198.51.100.20:1111"
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "pre-login-session"})
	rr := httptest.NewRecorder()
	_, _, status, _, _ := svc.LoginAPI(t.Context(), rr, req, user.Username, "correct-horse-battery", "login:user", []string{domain.RoleUser})
	if status != http.StatusOK {
		t.Fatalf("LoginAPI status=%d, want 200", status)
	}
	if oldSess, err := testStore.GetSession(t.Context(), "pre-login-session"); err == nil && oldSess != nil {
		t.Fatal("expected pre-login session to be revoked on LoginAPI")
	}

	for _, username := range []string{"bad1", "bad2"} {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
		r.RemoteAddr = "198.51.100.21:2222"
		w := httptest.NewRecorder()
		_, _, code, _, _ := svc.LoginAPI(t.Context(), w, r, username, "wrong-password", "login:user", []string{domain.RoleUser})
		if code != http.StatusUnauthorized {
			t.Fatalf("LoginAPI(%s) status=%d, want 401", username, code)
		}
	}
	r3 := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	r3.RemoteAddr = "198.51.100.21:2222"
	w3 := httptest.NewRecorder()
	_, _, code, _, _ := svc.LoginAPI(t.Context(), w3, r3, "bad3", "wrong-password", "login:user", []string{domain.RoleUser})
	if code != http.StatusTooManyRequests {
		t.Fatalf("third LoginAPI attempt status=%d, want 429", code)
	}
}

func TestAdminAndWorkspaceLogin_ShareAbuseScope(t *testing.T) {
	clearAuthTables()
	createTestUser("admin1", "admin1@example.com", "correct-horse-battery", domain.RoleAdmin)
	svc := newTestService(Config{
		AuthAbuseMaxFailures: 2,
		AuthAbuseBackoffBase: time.Minute,
		AuthAbuseBackoffMax:  5 * time.Minute,
	})
	tmpl := template.Must(template.New("admin_login.html").Parse(`{{define "root_template"}}{{.Error}}{{end}}`))

	// Fail once on AdminLogin, then once on WorkspaceLogin -> 2 failures in shared scope locks out 3rd attempt.
	for i, handler := range []http.HandlerFunc{svc.AdminLogin(tmpl), svc.WorkspaceLogin(tmpl)} {
		form := url.Values{"username": {"admin1"}, "password": {"wrong-password"}}
		req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.RemoteAddr = "198.51.100.30:3333"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("step %d: status=%d, want 401", i, rr.Code)
		}
	}

	form := url.Values{"username": {"admin1"}, "password": {"correct-horse-battery"}}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "198.51.100.30:3333"
	rr := httptest.NewRecorder()
	svc.WorkspaceLogin(tmpl).ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected shared scope lockout (429), got %d", rr.Code)
	}
}

func TestSignUp_And_SignUpAPI_IPRateLimit(t *testing.T) {
	clearAuthTables()
	svc := newTestService(Config{
		AuthAbuseMaxFailures: 2,
		AuthAbuseBackoffBase: time.Minute,
		AuthAbuseBackoffMax:  5 * time.Minute,
	})
	tmpl := template.Must(template.New("signup.html").Parse(`{{define "root_template"}}{{.Error}}{{end}}`))
	handler := svc.SignUp(tmpl)

	for i := 0; i < 2; i++ {
		form := url.Values{"username": {""}, "email": {""}, "password": {""}}
		req := httptest.NewRequest(http.MethodPost, "/signup", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.RemoteAddr = "198.51.100.40:4444"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("SignUp attempt %d status=%d, want 400", i, rr.Code)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/signup", strings.NewReader("username=u&email=e&password=p"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "198.51.100.40:4444"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("SignUp 3rd attempt status=%d, want 429", rr.Code)
	}

	// SignUpAPI from the same IP is also rate limited.
	apiReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/signup", nil)
	apiReq.RemoteAddr = "198.51.100.40:4444"
	apiRR := httptest.NewRecorder()
	status, code, _ := svc.SignUpAPI(t.Context(), apiRR, apiReq, "validuser", "valid@example.com", "correct-horse-battery")
	if status != http.StatusTooManyRequests || code != "rate_limited" {
		t.Fatalf("SignUpAPI status=%d code=%q, want 429 rate_limited", status, code)
	}
}

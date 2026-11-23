package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kyambuthia/sokomoko/internal/app"
	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/domain"
)

type mockAPIAuth struct {
	stubAuthService
	loginFn  func(context.Context, http.ResponseWriter, *http.Request, string, string, string, []string) (*db.User, string, int, string, string)
	signupFn func(context.Context, http.ResponseWriter, *http.Request, string, string, string) (*db.User, string, int, string, string)
	logoutFn func(context.Context, http.ResponseWriter, *http.Request)
}

func (m *mockAPIAuth) LoginAPI(ctx context.Context, w http.ResponseWriter, r *http.Request, u, p, scope string, roles []string) (*db.User, string, int, string, string) {
	return m.loginFn(ctx, w, r, u, p, scope, roles)
}
func (m *mockAPIAuth) SignUpAPI(ctx context.Context, w http.ResponseWriter, r *http.Request, u, e, p string) (*db.User, string, int, string, string) {
	return m.signupFn(ctx, w, r, u, e, p)
}
func (m *mockAPIAuth) LogoutAPI(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	if m.logoutFn != nil {
		m.logoutFn(ctx, w, r)
	}
}

func TestAPIAuth_LoginSignupLogout(t *testing.T) {
	ma := &mockAPIAuth{
		loginFn: func(_ context.Context, _ http.ResponseWriter, _ *http.Request, u, p, scope string, roles []string) (*db.User, string, int, string, string) {
			if scope != "login:workspace" || len(roles) != 2 {
				t.Fatalf("unexpected scope=%q roles=%v", scope, roles)
			}
			return &db.User{ID: 7, Username: u, Email: "a@b.c", Role: domain.RoleAdmin}, "csrf-123", http.StatusOK, "", ""
		},
		signupFn: func(_ context.Context, _ http.ResponseWriter, _ *http.Request, u, e, p string) (*db.User, string, int, string, string) {
			return &db.User{ID: 8, Username: u, Email: e, Role: domain.RoleUser}, "signup-csrf-456", http.StatusCreated, "", ""
		},
	}
	mux := http.NewServeMux()
	Register(app.New(app.Dependencies{Auth: ma}), mux)

	loginReq := httptest.NewRequest(http.MethodPost, apiPrefix+"/auth/login", strings.NewReader(`{"username":"qaadmin","password":"secret"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginReq.Host = "admin.localhost:6969"
	loginRec := httptest.NewRecorder()
	mux.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK || !strings.Contains(loginRec.Body.String(), "csrf-123") {
		t.Fatalf("login status=%d body=%s", loginRec.Code, loginRec.Body.String())
	}

	signupReq := httptest.NewRequest(http.MethodPost, apiPrefix+"/auth/signup", strings.NewReader(`{"username":"newbuyer","email":"n@b.c","password":"LongPassword123"}`))
	signupReq.Header.Set("Content-Type", "application/json")
	signupRec := httptest.NewRecorder()
	mux.ServeHTTP(signupRec, signupReq)
	if signupRec.Code != http.StatusCreated || !strings.Contains(signupRec.Body.String(), "signup-csrf-456") {
		t.Fatalf("signup status=%d body=%s", signupRec.Code, signupRec.Body.String())
	}

	demoMux := http.NewServeMux()
	Register(app.New(app.Dependencies{Auth: ma, DemoMode: true}), demoMux)
	demoSignup := httptest.NewRequest(http.MethodPost, apiPrefix+"/auth/signup", strings.NewReader(`{"username":"x","email":"x@y.z","password":"LongPassword123"}`))
	demoSignup.Header.Set("Content-Type", "application/json")
	demoRec := httptest.NewRecorder()
	demoMux.ServeHTTP(demoRec, demoSignup)
	if demoRec.Code != http.StatusForbidden {
		t.Fatalf("demo signup status=%d, want 403", demoRec.Code)
	}
}

package routes

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/kyambuthia/sokomoko/internal/app"
	"github.com/kyambuthia/sokomoko/internal/auth"
	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/domain"
	accountsvc "github.com/kyambuthia/sokomoko/internal/service/account"
	"github.com/kyambuthia/sokomoko/internal/ui"
)

type stubAccountAuth struct {
	app.AuthService
	tokens    []db.APIToken
	createErr error
	revokeOK  bool
	revokeErr error
	lastID    int64
}

func (s *stubAccountAuth) CSRFToken(_ *http.Request) string { return "csrf-test" }

func (s *stubAccountAuth) ListAPITokens(_ context.Context, _ int) ([]db.APIToken, error) {
	return s.tokens, nil
}

func (s *stubAccountAuth) CreateAPIToken(_ context.Context, userID int, name, clientType string, expiresAt sql.NullTime) (*db.APIToken, string, error) {
	if s.createErr != nil {
		return nil, "", s.createErr
	}
	raw := "smk_test_raw_token_abcdef"
	tok := &db.APIToken{
		ID:          42,
		UserID:      userID,
		Name:        name,
		TokenPrefix: raw[:12],
		ClientType:  clientType,
		ExpiresAt:   expiresAt,
		CreatedAt:   time.Now().UTC(),
	}
	s.tokens = append([]db.APIToken{*tok}, s.tokens...)
	return tok, raw, nil
}

func (s *stubAccountAuth) RevokeAPIToken(_ context.Context, _ int, tokenID int64) (bool, error) {
	s.lastID = tokenID
	if s.revokeErr != nil {
		return false, s.revokeErr
	}
	return s.revokeOK, nil
}

type stubAccountService struct {
	app.AccountService
}

func (s *stubAccountService) OrdersForUserPage(_ context.Context, _ int, _, _ int) ([]accountsvc.Order, int, error) {
	return nil, 0, nil
}

func TestAccountTokenCreateAndOneTimeDisplay(t *testing.T) {
	tmpls, err := ui.ParseTemplatesWithDemo(false)
	if err != nil {
		t.Fatalf("ParseTemplatesWithDemo: %v", err)
	}
	authSvc := &stubAccountAuth{revokeOK: true}
	a := &app.App{
		Auth:      authSvc,
		Account:   &stubAccountService{},
		Templates: tmpls,
	}

	form := url.Values{
		"name":            {"my-agent"},
		"client_type":     {"agent"},
		"expires_in_days": {"30"},
	}
	req := httptest.NewRequest(http.MethodPost, "/account/tokens", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(auth.ContextWithUser(req.Context(), &db.User{ID: 7, Username: "alice", Email: "alice@example.com", Role: domain.RoleUser}))
	rec := httptest.NewRecorder()

	AccountTokenCreate(a)(rec, req)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/account" {
		t.Fatalf("POST /account/tokens code=%d location=%q, want 302 /account", rec.Code, rec.Header().Get("Location"))
	}

	cookies := rec.Result().Cookies()
	getReq := httptest.NewRequest(http.MethodGet, "/account", nil)
	for _, c := range cookies {
		getReq.AddCookie(c)
	}
	getReq = getReq.WithContext(auth.ContextWithUser(getReq.Context(), &db.User{ID: 7, Username: "alice", Email: "alice@example.com", Role: domain.RoleUser}))
	getRec := httptest.NewRecorder()

	Account(a)(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET /account code=%d, want 200", getRec.Code)
	}
	body := getRec.Body.String()
	if !strings.Contains(body, "smk_test_raw_token_abcdef") {
		t.Fatalf("expected raw token to be shown once on /account")
	}
	if !strings.Contains(body, "my-agent") {
		t.Fatalf("expected token name in /account response")
	}

	// Second GET without re-setting cookie should not show the raw token.
	getReq2 := httptest.NewRequest(http.MethodGet, "/account", nil)
	getReq2 = getReq2.WithContext(auth.ContextWithUser(getReq2.Context(), &db.User{ID: 7, Username: "alice", Email: "alice@example.com", Role: domain.RoleUser}))
	getRec2 := httptest.NewRecorder()
	Account(a)(getRec2, getReq2)
	if strings.Contains(getRec2.Body.String(), "smk_test_raw_token_abcdef") {
		t.Fatalf("raw token must only be shown once")
	}
}

func TestAccountTokenRevokeAndDemoReadOnly(t *testing.T) {
	authSvc := &stubAccountAuth{revokeOK: true}
	a := &app.App{Auth: authSvc}

	mux := http.NewServeMux()
	mux.Handle("/account/tokens/{id}/revoke", demoReadOnly(a, "/account", AccountTokenRevoke(a)))

	req := httptest.NewRequest(http.MethodPost, "/account/tokens/42/revoke", nil)
	req = req.WithContext(auth.ContextWithUser(req.Context(), &db.User{ID: 7, Role: domain.RoleUser}))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/account" {
		t.Fatalf("POST /account/tokens/42/revoke code=%d location=%q", rec.Code, rec.Header().Get("Location"))
	}
	if authSvc.lastID != 42 {
		t.Fatalf("RevokeAPIToken id=%d, want 42", authSvc.lastID)
	}

	// Demo mode blocks POST mutations.
	a.DemoMode = true
	demoMux := http.NewServeMux()
	demoMux.Handle("/account/tokens", demoReadOnly(a, "/account", AccountTokenCreate(a)))
	demoMux.Handle("/account/tokens/{id}/revoke", demoReadOnly(a, "/account", AccountTokenRevoke(a)))

	for _, target := range []string{"/account/tokens", "/account/tokens/42/revoke"} {
		dReq := httptest.NewRequest(http.MethodPost, target, nil)
		dReq = dReq.WithContext(auth.ContextWithUser(dReq.Context(), &db.User{ID: 7, Role: domain.RoleUser}))
		dRec := httptest.NewRecorder()
		demoMux.ServeHTTP(dRec, dReq)
		if dRec.Code != http.StatusFound || dRec.Header().Get("Location") != "/account" {
			t.Fatalf("demo POST %s code=%d location=%q, want 302 /account", target, dRec.Code, dRec.Header().Get("Location"))
		}
	}
}

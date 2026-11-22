package auth

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/domain"
)

func TestAPIAuthMiddlewareAndCSRFWithBearerTokens(t *testing.T) {
	clearAuthTables()
	svc := newTestService(Config{})

	user := createTestUser("beareruser", "bearer@example.com", "password1234", domain.RoleUser)
	rawToken := "smk_valid_test_token_12345"
	tok, err := testStore.CreateAPIToken(t.Context(), user.ID, "agent", "smk_vali", rawToken, "agent", sql.NullTime{})
	if err != nil {
		t.Fatalf("CreateAPIToken: %v", err)
	}

	expiredRaw := "smk_expired_test_token_12345"
	if _, err := testStore.CreateAPIToken(t.Context(), user.ID, "expired", "smk_expi", expiredRaw, "agent", sql.NullTime{
		Time:  time.Now().Add(-time.Minute),
		Valid: true,
	}); err != nil {
		t.Fatalf("CreateAPIToken expired: %v", err)
	}

	revokedRaw := "smk_revoked_test_token_12345"
	revokedTok, err := testStore.CreateAPIToken(t.Context(), user.ID, "revoked", "smk_revo", revokedRaw, "agent", sql.NullTime{})
	if err != nil {
		t.Fatalf("CreateAPIToken revoked: %v", err)
	}
	if ok, err := testStore.RevokeAPIToken(t.Context(), user.ID, revokedTok.ID); err != nil || !ok {
		t.Fatalf("RevokeAPIToken: ok=%v err=%v", ok, err)
	}

	protected := svc.CSRFMiddleware(svc.APIAuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := GetUserFromContext(r.Context())
		if u == nil || u.ID != user.ID {
			t.Fatalf("unexpected user in context: %+v", u)
		}
		if ctxTok := GetAPITokenFromContext(r.Context()); ctxTok == nil || ctxTok.ID != tok.ID {
			t.Fatalf("unexpected api token in context: %+v", ctxTok)
		}
		w.WriteHeader(http.StatusOK)
	})))

	// 1. Valid Bearer token succeeds on state-changing API request without CSRF.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cart/items", nil)
	req.Header.Set("Authorization", "Bearer "+rawToken)
	rr := httptest.NewRecorder()
	protected.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("valid bearer POST status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}

	// Verify last_used_at was touched.
	tokens, err := testStore.ListAPITokensByUser(t.Context(), user.ID)
	if err != nil {
		t.Fatalf("ListAPITokensByUser: %v", err)
	}
	var foundTouch bool
	for _, item := range tokens {
		if item.ID == tok.ID && item.LastUsedAt.Valid {
			foundTouch = true
		}
	}
	if !foundTouch {
		t.Fatalf("expected token LastUsedAt to be updated")
	}

	// 2. Invalid, revoked, expired, and non-smk Bearer tokens return 401 invalid_token.
	for _, bad := range []string{
		"Bearer dummy",
		"Bearer smk_does_not_exist",
		"Bearer " + expiredRaw,
		"Bearer " + revokedRaw,
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/cart/items", nil)
		req.Header.Set("Authorization", bad)
		rr := httptest.NewRecorder()
		protected.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("bad auth %q status = %d, want 401; body=%s", bad, rr.Code, rr.Body.String())
		}
		var env apiErrorEnvelope
		if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
			t.Fatalf("decode error for %q: %v", bad, err)
		}
		if env.Error.Code != "invalid_token" {
			t.Fatalf("bad auth %q error code = %q, want invalid_token", bad, env.Error.Code)
		}
	}

	// 3. Dummy or invalid Bearer + valid session cookie without CSRF must still enforce CSRF.
	sess, err := svc.newSession(user.ID)
	if err != nil {
		t.Fatalf("newSession: %v", err)
	}
	if err := testStore.CreateSession(t.Context(), db.Session{
		ID:        sess.ID,
		UserID:    sess.UserID,
		CSRFToken: sess.CSRFToken,
		ExpiresAt: sess.ExpiresAt,
	}); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	for _, dummyAuth := range []string{"Bearer dummy", "Bearer smk_invalid_token"} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/cart/items", nil)
		req.Header.Set("Authorization", dummyAuth)
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sess.ID})
		rr := httptest.NewRecorder()
		protected.ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Fatalf("%q + cookie without CSRF status = %d, want 403; body=%s", dummyAuth, rr.Code, rr.Body.String())
		}
		var env apiErrorEnvelope
		if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
			t.Fatalf("decode csrf error: %v", err)
		}
		if env.Error.Code != "csrf_token_missing" {
			t.Fatalf("%q + cookie error code = %q, want csrf_token_missing", dummyAuth, env.Error.Code)
		}
	}

	// 4. Valid Bearer token on non-API route does NOT bypass CSRF.
	reqNonAPI := httptest.NewRequest(http.MethodPost, "/cart/add", nil)
	reqNonAPI.Header.Set("Authorization", "Bearer "+rawToken)
	reqNonAPI.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sess.ID})
	rrNonAPI := httptest.NewRecorder()
	protected.ServeHTTP(rrNonAPI, reqNonAPI)
	if rrNonAPI.Code != http.StatusForbidden {
		t.Fatalf("non-API bearer + cookie without CSRF status = %d, want 403", rrNonAPI.Code)
	}
}

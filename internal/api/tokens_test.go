package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kyambuthia/sokomoko/internal/app"
	"github.com/kyambuthia/sokomoko/internal/auth"
	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/domain"
)

type stubTokenAuthService struct {
	stubAuthService
	createFn func(context.Context, int, string, string, sql.NullTime) (*db.APIToken, string, error)
	listFn   func(context.Context, int) ([]db.APIToken, error)
	revokeFn func(context.Context, int, int64) (bool, error)
}

func (s *stubTokenAuthService) CreateAPIToken(ctx context.Context, userID int, name, clientType string, expiresAt sql.NullTime) (*db.APIToken, string, error) {
	return s.createFn(ctx, userID, name, clientType, expiresAt)
}

func (s *stubTokenAuthService) ListAPITokens(ctx context.Context, userID int) ([]db.APIToken, error) {
	return s.listFn(ctx, userID)
}

func (s *stubTokenAuthService) RevokeAPIToken(ctx context.Context, userID int, tokenID int64) (bool, error) {
	return s.revokeFn(ctx, userID, tokenID)
}

func TestMeTokensEndpoints(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	authSvc := &stubTokenAuthService{
		stubAuthService: stubAuthService{user: &auth.User{ID: 42, Username: "agentowner", Role: domain.RoleUser}},
		createFn: func(_ context.Context, uid int, name, clientType string, exp sql.NullTime) (*db.APIToken, string, error) {
			if uid != 42 || name != "deploy-bot" || clientType != "agent" || !exp.Valid {
				t.Fatalf("unexpected create args: uid=%d name=%q clientType=%q exp=%+v", uid, name, clientType, exp)
			}
			return &db.APIToken{
				ID:          7,
				UserID:      uid,
				Name:        name,
				TokenPrefix: "smk_testpref",
				ClientType:  clientType,
				ExpiresAt:   exp,
				CreatedAt:   now,
			}, "smk_testprefix_raw_secret", nil
		},
		listFn: func(_ context.Context, uid int) ([]db.APIToken, error) {
			return []db.APIToken{{
				ID:          7,
				UserID:      uid,
				Name:        "deploy-bot",
				TokenPrefix: "smk_testpref",
				ClientType:  "agent",
				CreatedAt:   now,
			}}, nil
		},
		revokeFn: func(_ context.Context, uid int, id int64) (bool, error) {
			return uid == 42 && id == 7, nil
		},
	}

	mux := http.NewServeMux()
	Register(app.New(app.Dependencies{Auth: authSvc}), mux)

	// POST /api/v1/me/tokens requires JSON content-type.
	nonJSONReq := httptest.NewRequest(http.MethodPost, apiPrefix+"/me/tokens", strings.NewReader(`{"name":"deploy-bot"}`))
	nonJSONRec := httptest.NewRecorder()
	mux.ServeHTTP(nonJSONRec, nonJSONReq)
	if nonJSONRec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("non-json POST status=%d, want 415", nonJSONRec.Code)
	}

	// POST /api/v1/me/tokens returns 201 and raw token once.
	createReq := httptest.NewRequest(http.MethodPost, apiPrefix+"/me/tokens", strings.NewReader(`{"name":"deploy-bot","expires_in_days":30}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	mux.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("POST status=%d, want 201; body=%s", createRec.Code, createRec.Body.String())
	}
	var createResp struct {
		Data struct {
			ID          int64  `json:"id"`
			Name        string `json:"name"`
			TokenPrefix string `json:"token_prefix"`
			Token       string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if createResp.Data.ID != 7 || createResp.Data.Token != "smk_testprefix_raw_secret" {
		t.Fatalf("unexpected create data: %+v", createResp.Data)
	}

	// GET /api/v1/me/tokens lists tokens without raw secret.
	listReq := httptest.NewRequest(http.MethodGet, apiPrefix+"/me/tokens", nil)
	listRec := httptest.NewRecorder()
	mux.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK || strings.Contains(listRec.Body.String(), "raw_secret") {
		t.Fatalf("GET status=%d body=%s", listRec.Code, listRec.Body.String())
	}

	// DELETE /api/v1/me/tokens/7 revokes owned token.
	delReq := httptest.NewRequest(http.MethodDelete, apiPrefix+"/me/tokens/7", nil)
	delRec := httptest.NewRecorder()
	mux.ServeHTTP(delRec, delReq)
	if delRec.Code != http.StatusOK {
		t.Fatalf("DELETE status=%d, want 200; body=%s", delRec.Code, delRec.Body.String())
	}

	// DELETE /api/v1/me/tokens/999 returns 404.
	notFoundReq := httptest.NewRequest(http.MethodDelete, apiPrefix+"/me/tokens/999", nil)
	notFoundRec := httptest.NewRecorder()
	mux.ServeHTTP(notFoundRec, notFoundReq)
	if notFoundRec.Code != http.StatusNotFound {
		t.Fatalf("DELETE missing status=%d, want 404", notFoundRec.Code)
	}

	// Demo mode blocks POST and DELETE on /me/tokens.
	demoMux := http.NewServeMux()
	Register(app.New(app.Dependencies{Auth: authSvc, DemoMode: true}), demoMux)
	demoPost := httptest.NewRequest(http.MethodPost, apiPrefix+"/me/tokens", strings.NewReader(`{"name":"deploy-bot"}`))
	demoPost.Header.Set("Content-Type", "application/json")
	demoPostRec := httptest.NewRecorder()
	demoMux.ServeHTTP(demoPostRec, demoPost)
	if demoPostRec.Code != http.StatusForbidden {
		t.Fatalf("demo POST status=%d, want 403", demoPostRec.Code)
	}
	demoDel := httptest.NewRequest(http.MethodDelete, apiPrefix+"/me/tokens/7", nil)
	demoDelRec := httptest.NewRecorder()
	demoMux.ServeHTTP(demoDelRec, demoDel)
	if demoDelRec.Code != http.StatusForbidden {
		t.Fatalf("demo DELETE status=%d, want 403", demoDelRec.Code)
	}
}

func TestOpenAPIIncludesBearerAuthAndMeTokens(t *testing.T) {
	mux := http.NewServeMux()
	Register(app.New(app.Dependencies{Auth: &stubAuthService{}}), mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, apiPrefix+"/openapi.json", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200", rec.Code)
	}
	var doc struct {
		Paths      map[string]map[string]any `json:"paths"`
		Components struct {
			SecuritySchemes map[string]any `json:"securitySchemes"`
		} `json:"components"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if doc.Components.SecuritySchemes["bearerAuth"] == nil {
		t.Fatal("expected components.securitySchemes.bearerAuth")
	}
	if doc.Paths["/api/v1/me/tokens"] == nil || doc.Paths["/api/v1/me/tokens/{id}"] == nil {
		t.Fatal("expected /api/v1/me/tokens and /api/v1/me/tokens/{id} in openapi.json")
	}
}

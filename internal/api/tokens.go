package api

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kyambuthia/sokomoko/internal/db"
)

type apiTokenDTO struct {
	ID          int64      `json:"id"`
	Name        string     `json:"name"`
	TokenPrefix string     `json:"token_prefix"`
	ClientType  string     `json:"client_type"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

func mapAPIToken(tok db.APIToken) apiTokenDTO {
	dto := apiTokenDTO{
		ID:          tok.ID,
		Name:        tok.Name,
		TokenPrefix: tok.TokenPrefix,
		ClientType:  tok.ClientType,
		CreatedAt:   tok.CreatedAt,
	}
	if tok.LastUsedAt.Valid {
		t := tok.LastUsedAt.Time
		dto.LastUsedAt = &t
	}
	if tok.ExpiresAt.Valid {
		t := tok.ExpiresAt.Time
		dto.ExpiresAt = &t
	}
	if tok.RevokedAt.Valid {
		t := tok.RevokedAt.Time
		dto.RevokedAt = &t
	}
	return dto
}

func (h *Handler) meTokens(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication_required", "Authentication is required")
		return
	}

	switch r.Method {
	case http.MethodGet:
		tokens, err := h.app.Auth.ListAPITokens(r.Context(), uid)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "tokens_unavailable", "Unable to load API tokens")
			return
		}
		items := make([]apiTokenDTO, 0, len(tokens))
		for _, tok := range tokens {
			items = append(items, mapAPIToken(tok))
		}
		writeData(w, http.StatusOK, struct {
			Items []apiTokenDTO `json:"items"`
		}{Items: items})
	case http.MethodPost:
		var input struct {
			Name          string `json:"name"`
			ClientType    string `json:"client_type"`
			ExpiresInDays *int   `json:"expires_in_days"`
		}
		if err := decodeJSON(r, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON")
			return
		}
		name := strings.TrimSpace(input.Name)
		if len(name) == 0 || len(name) > 100 {
			writeError(w, http.StatusBadRequest, "invalid_token_name", "Token name must be 1 to 100 characters")
			return
		}
		clientType := strings.TrimSpace(input.ClientType)
		if clientType == "" {
			clientType = "agent"
		}
		if len(clientType) > 32 {
			writeError(w, http.StatusBadRequest, "invalid_client_type", "Client type must be at most 32 characters")
			return
		}
		var expiresAt sql.NullTime
		if input.ExpiresInDays != nil {
			if *input.ExpiresInDays <= 0 || *input.ExpiresInDays > 3650 {
				writeError(w, http.StatusBadRequest, "invalid_expiry", "expires_in_days must be between 1 and 3650")
				return
			}
			expiresAt = sql.NullTime{
				Time:  time.Now().Add(time.Duration(*input.ExpiresInDays) * 24 * time.Hour),
				Valid: true,
			}
		}

		tok, rawToken, err := h.app.Auth.CreateAPIToken(r.Context(), uid, name, clientType, expiresAt)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "token_create_failed", "Unable to create API token")
			return
		}
		writeData(w, http.StatusCreated, struct {
			apiTokenDTO
			Token string `json:"token"`
		}{
			apiTokenDTO: mapAPIToken(*tok),
			Token:       rawToken,
		})
	default:
		methodNotAllowed(w, http.MethodGet, http.MethodPost)
	}
}

func (h *Handler) meToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		methodNotAllowed(w, http.MethodDelete)
		return
	}
	uid, ok := userID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication_required", "Authentication is required")
		return
	}
	rawID := strings.TrimSpace(r.PathValue("id"))
	if rawID == "" {
		rawID = pathTail(r.URL.Path, apiPrefix+"/me/tokens/")
	}
	tokenID, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || tokenID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_token_id", "Token id must be a positive integer")
		return
	}

	revoked, err := h.app.Auth.RevokeAPIToken(r.Context(), uid, tokenID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token_revoke_failed", "Unable to revoke API token")
		return
	}
	if !revoked {
		writeError(w, http.StatusNotFound, "token_not_found", "API token not found")
		return
	}
	writeData(w, http.StatusOK, struct {
		ID      int64 `json:"id"`
		Revoked bool  `json:"revoked"`
	}{ID: tokenID, Revoked: true})
}

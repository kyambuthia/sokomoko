package api

import (
	"net"
	"net/http"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/auth"
	"github.com/kyambuthia/sokomoko/internal/domain"
)

func loginScopeForHost(r *http.Request) (string, []string) {
	host := strings.ToLower(r.Host)
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	switch {
	case strings.HasPrefix(host, "admin."):
		return "login:admin", []string{domain.RoleAdmin, domain.RoleStaff}
	case strings.HasPrefix(host, "partner."):
		return "login:workspace", []string{domain.RoleAdmin, domain.RoleStaff, domain.RolePartner}
	case strings.HasPrefix(host, "courier."):
		return "login:courier", []string{domain.RoleCourier}
	default:
		return "login:user", []string{domain.RoleUser}
	}
}

func (h *Handler) authLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON")
		return
	}
	scope, roles := loginScopeForHost(r)
	user, csrfToken, status, errCode, errMsg := h.app.Auth.LoginAPI(r.Context(), w, r, input.Username, input.Password, scope, roles)
	if errCode != "" {
		writeError(w, status, errCode, errMsg)
		return
	}
	writeData(w, http.StatusOK, struct {
		ID        int    `json:"id"`
		Username  string `json:"username"`
		Email     string `json:"email"`
		Role      string `json:"role"`
		CSRFToken string `json:"csrf_token"`
	}{ID: user.ID, Username: user.Username, Email: user.Email, Role: user.Role, CSRFToken: csrfToken})
}

func (h *Handler) authSignUp(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if h.app.DemoMode {
		writeError(w, http.StatusForbidden, "demo_mode", "Sign up is disabled in demo mode")
		return
	}
	var input struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON")
		return
	}
	status, errCode, errMsg := h.app.Auth.SignUpAPI(r.Context(), input.Username, input.Email, input.Password)
	if errCode != "" {
		writeError(w, status, errCode, errMsg)
		return
	}
	writeData(w, http.StatusCreated, map[string]string{"username": strings.TrimSpace(input.Username)})
}

func (h *Handler) authLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	h.app.Auth.LogoutAPI(r.Context(), w, r)
	writeData(w, http.StatusOK, map[string]bool{"logged_out": true})
}

func (h *Handler) mePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		methodNotAllowed(w, http.MethodPatch)
		return
	}
	user := auth.GetUserFromContext(r.Context())
	var input struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON")
		return
	}
	csrfToken, status, errCode, errMsg := h.app.Auth.ChangePasswordAPI(r.Context(), w, r, user, input.CurrentPassword, input.NewPassword)
	if errCode != "" {
		writeError(w, status, errCode, errMsg)
		return
	}
	writeData(w, http.StatusOK, map[string]string{"status": "changed", "csrf_token": csrfToken})
}

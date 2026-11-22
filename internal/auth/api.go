package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/domain"
)

// LoginAPI authenticates credentials for JSON API clients, creates a session,
// sets the session cookie, and returns the user and CSRF token.
func (s *Service) LoginAPI(ctx context.Context, w http.ResponseWriter, r *http.Request, username, password, abuseScope string, allowedRoles []string) (*db.User, string, int, string, string) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return nil, "", http.StatusBadRequest, "invalid_request", "Username and password are required"
	}
	ipLocked, ipGeneration := s.checkAbuseLockByIP(w, abuseScope, r)
	if ipLocked {
		return nil, "", http.StatusTooManyRequests, "rate_limited", "Too many attempts. Please wait and try again."
	}
	locked, abuseGeneration := s.checkAbuseLock(w, abuseScope, username, false, r)
	if locked {
		return nil, "", http.StatusTooManyRequests, "rate_limited", "Too many attempts. Please wait and try again."
	}

	user, err := s.findUserByIdentifier(ctx, username)
	if err != nil {
		slog.ErrorContext(ctx, "api login lookup failed", "error", err)
		return nil, "", http.StatusInternalServerError, "login_failed", "Unable to sign in"
	}
	if user != nil && len(allowedRoles) > 0 && !containsRole(allowedRoles, user.Role) {
		user = nil
	}
	if !checkPassword(user, password) {
		s.markAbuseFailure(abuseScope, username, false, r, abuseGeneration)
		s.recordAbuseAttemptByIP(abuseScope, r, ipGeneration)
		return nil, "", http.StatusUnauthorized, "invalid_credentials", "Invalid credentials"
	}
	s.clearAbuseFailures(abuseScope, username, false, r)

	if user.Role == domain.RolePartner {
		partnerID, err := s.store.PartnerIDForUser(ctx, user.ID)
		if err != nil {
			slog.ErrorContext(ctx, "api partner lookup failed", "error", err)
			return nil, "", http.StatusInternalServerError, "login_failed", "Unable to sign in"
		}
		if partnerID == 0 {
			return nil, "", http.StatusForbidden, "account_suspended", "This vendor account is suspended. Contact the marketplace team."
		}
	}

	s.revokeRequestSessions(ctx, r, "api sign-in: revoke previous session")
	session, err := s.newSession(user.ID)
	if err != nil {
		return nil, "", http.StatusInternalServerError, "login_failed", "Unable to create session"
	}
	if err := s.store.CreateSessionForPasswordHash(ctx, session, user.PasswordHash); err != nil {
		if errors.Is(err, db.ErrUserInactive) || errors.Is(err, db.ErrPasswordChanged) {
			s.clearSessionCookie(w, r)
			return nil, "", http.StatusUnauthorized, "invalid_credentials", "Invalid credentials"
		}
		slog.ErrorContext(ctx, "api create session failed", "error", err)
		return nil, "", http.StatusInternalServerError, "login_failed", "Unable to create session"
	}
	s.setSessionCookie(w, r, session)
	return user, session.CSRFToken, http.StatusOK, "", ""
}

// SignUpAPI registers a new customer account via JSON API.
func (s *Service) SignUpAPI(ctx context.Context, w http.ResponseWriter, r *http.Request, username, email, password string) (int, string, string) {
	const signupScope = "signup"
	ipLocked, ipGeneration := s.checkAbuseLockByIP(w, signupScope, r)
	if ipLocked {
		return http.StatusTooManyRequests, "rate_limited", "Too many attempts. Please wait and try again."
	}
	s.recordAbuseAttemptByIP(signupScope, r, ipGeneration)

	username = strings.TrimSpace(username)
	email = normalizeEmail(email)
	switch {
	case username == "" || email == "" || password == "":
		return http.StatusBadRequest, "invalid_request", "Username, email, and password are required"
	case !isValidUsername(username):
		return http.StatusBadRequest, "invalid_username", "Username must be 3-32 characters and use letters, numbers, dots, dashes, or underscores"
	case !isValidEmail(email):
		return http.StatusBadRequest, "invalid_email", "Enter a valid email address"
	case !isStrongEnoughPassword(password):
		return http.StatusBadRequest, "invalid_password", "Password must be 10 to 72 characters"
	}
	if _, err := createUserWithPassword(ctx, s.store, username, email, password, domain.RoleUser); err != nil {
		statusCode, msg := mapAccountCreationError(err, "Unable to create account")
		code := "signup_failed"
		if statusCode == http.StatusConflict {
			code = "account_exists"
		}
		return statusCode, code, msg
	}
	return http.StatusCreated, "", ""
}

// LogoutAPI revokes the active session and clears the session cookie.
func (s *Service) LogoutAPI(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	s.revokeRequestSessions(ctx, r, "api logout")
	s.clearSessionCookie(w, r)
}

// ChangePasswordAPI verifies the current password, updates the hash, rotates
// the session cookie, and returns the new CSRF token.
func (s *Service) ChangePasswordAPI(ctx context.Context, w http.ResponseWriter, r *http.Request, user *db.User, currentPassword, newPassword string) (string, int, string, string) {
	if user == nil {
		return "", http.StatusUnauthorized, "authentication_required", "Authentication is required"
	}
	const scope = "password_change"
	identifier := strconv.Itoa(user.ID)
	locked, abuseGeneration := s.checkAbuseLock(w, scope, identifier, true, r)
	if locked {
		return "", http.StatusTooManyRequests, "rate_limited", "Too many attempts. Please wait and try again."
	}
	switch {
	case currentPassword == "" || newPassword == "":
		return "", http.StatusBadRequest, "invalid_request", "Current password and new password are required"
	case !checkPassword(user, currentPassword):
		s.markAbuseFailure(scope, identifier, true, r, abuseGeneration)
		return "", http.StatusBadRequest, "invalid_password", "Current password is incorrect"
	case !isStrongEnoughPassword(newPassword):
		return "", http.StatusBadRequest, "invalid_password", "Password must be 10 to 72 characters"
	case newPassword == currentPassword:
		return "", http.StatusBadRequest, "invalid_password", "Choose a password you have not been using"
	}
	hash, err := hashPassword(newPassword)
	if err != nil {
		return "", http.StatusInternalServerError, "password_change_failed", "Unable to change password"
	}
	s.revokeRequestSessions(ctx, r, "api sign-in: revoke previous session")
	session, err := s.newSession(user.ID)
	if err != nil {
		return "", http.StatusInternalServerError, "password_change_failed", "Unable to change password"
	}
	if err := s.store.ChangeUserPassword(ctx, user.ID, user.PasswordHash, hash, session); err != nil {
		if errors.Is(err, db.ErrPasswordChanged) {
			return "", http.StatusConflict, "password_changed", "Your password changed in another request. Sign in again to continue."
		}
		slog.ErrorContext(ctx, "api password change failed", "user_id", user.ID, "error", err)
		return "", http.StatusInternalServerError, "password_change_failed", "Unable to change password"
	}
	s.clearAbuseFailures(scope, identifier, true, r)
	s.revokeRequestSessions(ctx, r, "api password change")
	s.setSessionCookie(w, r, session)
	return session.CSRFToken, http.StatusOK, "", ""
}

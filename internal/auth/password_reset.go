package auth

import (
	"context"
	"errors"
	"html/template"
	"log"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"time"
)

type PasswordResetEmailSender func(ctx context.Context, recipientEmail string, resetLink string) error

type PasswordResetRequestData struct {
	Title      string
	Heading    string
	Helper     string
	Identifier string
	Message    string
	Error      string
	ResetLink  string
	CSRFToken  string
}

type PasswordResetConfirmData struct {
	Title     string
	Heading   string
	Helper    string
	Token     string
	Message   string
	Error     string
	ShowForm  bool
	CSRFToken string
}

func (s *Service) shouldExposeResetLink() bool {
	return s.environment != "production"
}

func (s *Service) buildPasswordResetLink(r *http.Request, token string) string {
	relative := "/password-reset/confirm?token=" + url.QueryEscape(token)

	baseURL := strings.TrimSpace(s.passwordResetBaseURL)
	if baseURL != "" {
		parsed, err := url.Parse(baseURL)
		if err == nil && parsed.Scheme != "" && parsed.Host != "" {
			parsed.Path = strings.TrimSuffix(parsed.Path, "/") + "/password-reset/confirm"
			query := parsed.Query()
			query.Set("token", token)
			parsed.RawQuery = query.Encode()
			return parsed.String()
		}
	}

	host := ""
	if r != nil {
		host = strings.TrimSpace(r.Host)
	}
	if host == "" {
		return relative
	}

	return requestScheme(r) + "://" + host + relative
}

func (s *Service) dispatchPasswordResetEmail(recipientEmail string, resetLink string, userID int) {
	sender := s.passwordResetEmailSender
	if sender == nil {
		return
	}

	go func() {
		var err error
		backoff := s.resetEmailBackoff
		if backoff == nil {
			backoff = defaultResetEmailBackoff
		}
		for attempt := 0; attempt < len(backoff)+1; attempt++ {
			if attempt > 0 {
				time.Sleep(backoff[attempt-1])
			}
			ctx, cancel := context.WithTimeout(context.Background(), resetEmailTimeout)
			err = sender(ctx, recipientEmail, resetLink)
			cancel()
			if err == nil || permanentMailError(err) {
				break
			}
		}
		if err != nil {
			log.Printf("password reset email send failed for user_id=%d: %v", userID, err)
		}
	}()
}

// defaultResetEmailBackoff is the wait before each retry of a reset email.
var defaultResetEmailBackoff = []time.Duration{time.Second, 4 * time.Second}

// permanentMailError reports an SMTP 5xx reply, which a retry cannot fix.
func permanentMailError(err error) bool {
	var reply *textproto.Error
	return errors.As(err, &reply) && reply.Code >= 500
}

func (s *Service) PasswordResetRequest(tmpl *template.Template, allowedRoles []string, title string, helper string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data := PasswordResetRequestData{
			Title:     title,
			Heading:   title,
			Helper:    helper,
			CSRFToken: s.CSRFTokenForSession(r),
		}

		if r.Method == http.MethodGet {
			renderWithStatus(w, tmpl, 0, data)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		identifier := strings.TrimSpace(r.FormValue("identifier"))
		data.Identifier = identifier
		if identifier == "" {
			data.Error = "Enter username or email"
			renderWithStatus(w, tmpl, 0, data)
			return
		}

		if s.checkAbuseLock(w, "password-reset:request", identifier, false, r) {
			data.Error = "Too many attempts. Please wait and try again."
			renderWithStatus(w, tmpl, http.StatusTooManyRequests, data)
			return
		}

		user, err := s.findUserByIdentifier(r.Context(), identifier)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		data.Message = "If the account exists and is allowed in this area, a reset link is available below."
		if user != nil && containsRole(allowedRoles, user.Role) {
			resetToken, tokenErr := generateOpaqueToken(32)
			if tokenErr != nil {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
			if err := s.store.CreatePasswordResetToken(r.Context(), user.ID, resetToken, time.Now().Add(resetTokenDuration)); err != nil {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}

			resetLink := s.buildPasswordResetLink(r, resetToken)
			s.dispatchPasswordResetEmail(user.Email, resetLink, user.ID)
			if s.shouldExposeResetLink() {
				data.ResetLink = resetLink
			}
			s.clearAbuseFailures("password-reset:request", identifier, false, r)
		} else {
			s.markAbuseFailure("password-reset:request", identifier, false, r)
		}

		renderWithStatus(w, tmpl, 0, data)
	}
}

func (s *Service) PasswordResetConfirm(tmpl *template.Template, allowedRoles []string, title string, helper string, loginPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data := PasswordResetConfirmData{
			Title:     title,
			Heading:   title,
			Helper:    helper,
			ShowForm:  true,
			CSRFToken: s.CSRFTokenForSession(r),
		}

		if r.Method == http.MethodGet {
			token := strings.TrimSpace(r.URL.Query().Get("token"))
			data.Token = token
			abuseIdentifier := token
			if abuseIdentifier == "" {
				abuseIdentifier = "missing-token"
			}
			if s.checkAbuseLock(w, "password-reset:confirm", abuseIdentifier, true, r) {
				data.Error = "Too many attempts. Please wait and try again."
				renderWithStatus(w, tmpl, http.StatusTooManyRequests, data)
				return
			}
			if token == "" {
				s.markAbuseFailure("password-reset:confirm", abuseIdentifier, true, r)
				data.Error = "Missing reset token"
				renderWithStatus(w, tmpl, 0, data)
				return
			}

			resetToken, err := s.store.GetValidPasswordResetToken(r.Context(), token)
			if err != nil {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
			if resetToken == nil {
				s.markAbuseFailure("password-reset:confirm", abuseIdentifier, true, r)
				data.Error = "Reset token is invalid or expired"
				renderWithStatus(w, tmpl, 0, data)
				return
			}

			user, err := s.store.GetUserByID(r.Context(), resetToken.UserID)
			if err != nil || user == nil || !containsRole(allowedRoles, user.Role) {
				s.markAbuseFailure("password-reset:confirm", abuseIdentifier, true, r)
				data.Error = "Reset token is invalid for this account type"
				renderWithStatus(w, tmpl, 0, data)
				return
			}
			s.clearAbuseFailures("password-reset:confirm", abuseIdentifier, true, r)

			renderWithStatus(w, tmpl, 0, data)
			return
		}

		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		token := strings.TrimSpace(r.FormValue("token"))
		password := r.FormValue("password")
		confirmPassword := r.FormValue("confirm_password")
		data.Token = token
		abuseIdentifier := token
		if abuseIdentifier == "" {
			abuseIdentifier = "missing-token"
		}

		if s.checkAbuseLock(w, "password-reset:confirm", abuseIdentifier, true, r) {
			data.Error = "Too many attempts. Please wait and try again."
			renderWithStatus(w, tmpl, http.StatusTooManyRequests, data)
			return
		}

		if token == "" || password == "" || confirmPassword == "" {
			data.Error = "All fields are required"
			renderWithStatus(w, tmpl, 0, data)
			return
		}
		if password != confirmPassword {
			data.Error = "Passwords do not match"
			renderWithStatus(w, tmpl, 0, data)
			return
		}
		if !isStrongEnoughPassword(password) {
			data.Error = "Password must be 10 to 72 characters"
			renderWithStatus(w, tmpl, 0, data)
			return
		}

		resetToken, err := s.store.GetValidPasswordResetToken(r.Context(), token)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if resetToken == nil {
			s.markAbuseFailure("password-reset:confirm", abuseIdentifier, true, r)
			data.Error = "Reset token is invalid or expired"
			renderWithStatus(w, tmpl, 0, data)
			return
		}

		user, err := s.store.GetUserByID(r.Context(), resetToken.UserID)
		if err != nil || user == nil || !containsRole(allowedRoles, user.Role) {
			s.markAbuseFailure("password-reset:confirm", abuseIdentifier, true, r)
			data.Error = "Reset token is invalid for this account type"
			renderWithStatus(w, tmpl, 0, data)
			return
		}

		passwordHash, err := hashPassword(password)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		used, err := s.store.UsePasswordResetToken(r.Context(), token, passwordHash)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if !used {
			s.markAbuseFailure("password-reset:confirm", abuseIdentifier, true, r)
			data.Error = "Reset token is invalid or expired"
			renderWithStatus(w, tmpl, 0, data)
			return
		}
		s.clearAbuseFailures("password-reset:confirm", abuseIdentifier, true, r)

		data.ShowForm = false
		data.Message = "Password reset successful. You can now sign in at " + loginPath
		renderWithStatus(w, tmpl, 0, data)
	}
}

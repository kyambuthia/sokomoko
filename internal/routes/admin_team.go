package routes

import (
	"errors"
	"net/http"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/app"
	adminsvc "github.com/kyambuthia/sokomoko/internal/service/admin"
)

const teamPageSize = 50 // Number of accounts shown on each team page.

// AdminTeam lists users and allows admin-only staff/user deactivation.
func AdminTeam(a *app.App) http.HandlerFunc {
	svc := a.Admin

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		metrics, err := loadAdminMetrics(r.Context(), svc)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		page := adminTeamPage(metrics, adminRoleFromContext(r), nil)

		if r.Method == http.MethodPost {
			reactivate := r.FormValue("action") == "reactivate"
			var err error
			if reactivate {
				err = svc.ReactivateUser(r.Context(), adminActorFromContext(r), r.FormValue("user_id"))
			} else {
				err = svc.DeactivateUser(r.Context(), adminActorFromContext(r), r.FormValue("user_id"))
			}
			switch {
			case errors.Is(err, adminsvc.ErrForbiddenUserAction):
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			case errors.Is(err, adminsvc.ErrInvalidUserID):
				page.TeamError = "Invalid user ID"
			case errors.Is(err, adminsvc.ErrUserNotFound):
				page.TeamError = "User does not exist"
			case errors.Is(err, adminsvc.ErrProtectedUser):
				page.TeamError = "You cannot deactivate your own account or the last active admin"
			case errors.Is(err, adminsvc.ErrCourierHasActiveDeliveries):
				page.TeamError = "This courier has deliveries assigned or in transit. Reassign them before deactivating the account."
			case err != nil && reactivate:
				page.TeamError = "Unable to reactivate user"
			case err != nil:
				page.TeamError = "Unable to deactivate user"
			case reactivate:
				redirectWithFlash(w, r, "/team", "success", "User account reactivated")
				return
			default:
				redirectWithFlash(w, r, "/team", "success", "User account deactivated")
				return
			}
		} else if flash := popFlash(w, r); flash.Message != "" {
			page.TeamMessage = flash.Message
		}

		query := strings.TrimSpace(r.URL.Query().Get("q"))
		pageNum := pageParam(r)
		teamMembers, total, err := svc.TeamMembers(r.Context(), query, pageNum, teamPageSize)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		displayPage := min(pageNum, pageCount(teamPageSize, total))
		if r.Method == http.MethodGet && redirectPastLastPage(w, r, pageNum, teamPageSize, total) {
			return
		}
		if r.Method == http.MethodPost && page.TeamError != "" && total > 0 && displayPage != pageNum {
			teamMembers, total, err = svc.TeamMembers(r.Context(), query, displayPage, teamPageSize)
			if err != nil {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
		}
		page.TeamMembers = teamMembers
		page.TeamQuery = query
		page.TeamPager = newPager(r, displayPage, teamPageSize, total)
		if page.Deactivated, err = svc.DeactivatedMembers(r.Context()); err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		renderAdminPage(a, w, r, http.StatusOK, page)
	}
}

func AdminAudit(a *app.App) http.HandlerFunc {
	svc := a.Admin

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		metrics, err := loadAdminMetrics(r.Context(), svc)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		logs, err := svc.AuditLogs(r.Context(), 100)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		renderAdminPage(a, w, r, http.StatusOK, adminAuditPage(metrics, adminRoleFromContext(r), logs))
	}
}

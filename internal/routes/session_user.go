package routes

import (
	"net/http"

	"github.com/kyambuthia/sokomoko/internal/auth"
)

type requestUser struct {
	ID       int
	Role     string
	Username string
}

func requestUserFromContext(r *http.Request) *requestUser {
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		return nil
	}
	return &requestUser{ID: user.ID, Role: user.Role, Username: user.Username}
}

func requestUserID(r *http.Request) (int, bool) {
	user := requestUserFromContext(r)
	if user == nil {
		return 0, false
	}
	return user.ID, true
}

// Viewer describes the signed-in user, if any, for storefront templates.
type Viewer struct {
	SignedIn bool
	Username string
}

func viewerFromRequest(r *http.Request) Viewer {
	user := requestUserFromContext(r)
	if user == nil {
		return Viewer{}
	}
	return Viewer{SignedIn: true, Username: user.Username}
}

func methodNotAllowed(w http.ResponseWriter, allowed ...string) {
	for _, m := range allowed {
		w.Header().Add("Allow", m)
	}
	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

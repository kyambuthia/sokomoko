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

	return &requestUser{
		ID:       user.ID,
		Role:     user.Role,
		Username: user.Username,
	}
}

func requestUserID(r *http.Request) (int, bool) {
	user := requestUserFromContext(r)
	if user == nil {
		return 0, false
	}
	return user.ID, true
}

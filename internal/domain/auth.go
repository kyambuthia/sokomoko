package domain

import "time"

type Session struct {
	ID        string
	UserID    int
	CSRFToken string
	ExpiresAt time.Time
	CreatedAt time.Time
}

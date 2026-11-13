package directory

import "github.com/kyambuthia/sokomoko/internal/db"

type (
	ServiceListing    = db.ServiceListing
	ServiceFilter     = db.ServiceFilter
	ServiceWeeklyHour = db.ServiceWeeklyHour
)

var (
	ErrServiceNotFound = db.ErrServiceNotFound
	ErrServiceConflict = db.ErrServiceConflict
)

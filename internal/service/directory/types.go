package directory

import "github.com/kyambuthia/sokomoko/internal/domain"

type (
	ServiceListing    = domain.ServiceListing
	ServiceFilter     = domain.ServiceFilter
	ServiceWeeklyHour = domain.ServiceWeeklyHour
)

var (
	ErrServiceNotFound = domain.ErrServiceNotFound
	ErrServiceConflict = domain.ErrServiceConflict
)

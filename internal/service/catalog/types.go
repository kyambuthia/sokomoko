package catalog

import "github.com/kyambuthia/sokomoko/internal/domain"

type (
	ProductImage = domain.ProductImage
	ProductVideo = domain.ProductVideo
	Partner      = domain.Partner
)

var ErrProductNotFound = domain.ErrProductNotFound

package catalog

import "github.com/kyambuthia/sokomoko/internal/db"

type (
	ProductImage = db.ProductImage
	ProductVideo = db.ProductVideo
	Partner      = db.Partner
)

var ErrProductNotFound = db.ErrProductNotFound

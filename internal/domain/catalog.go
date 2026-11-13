package domain

import (
	"errors"
	"time"
)

var ErrProductNotFound = errors.New("product not found")

type Partner struct {
	ID           int
	Name         string
	Slug         string
	ContactEmail string
	Status       string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type ProductImage struct {
	ID           int
	ProductID    int
	URL          string
	AltText      string
	DisplayOrder int
	ContentType  string
	Data         []byte
	CreatedAt    time.Time
}

type ProductVideo struct {
	ProductID   int
	ContentType string
	Data        []byte
	CreatedAt   time.Time
}

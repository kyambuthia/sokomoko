package domain

import (
	"errors"
	"time"
)

var ErrInvalidCourierView = errors.New("invalid courier job view")

// CourierDelivery is a courier's operational view of an assigned delivery:
// customer name, delivery address, fulfillment state, and item quantities.
type CourierDelivery struct {
	ID              int
	OrderID         int
	CourierUserID   int
	CourierName     string
	AssignedByID    int
	CustomerName    string
	DeliveryAddress string
	OrderStatus     string
	PartnerStatus   string
	DeliveryStatus  string
	Status          string
	FailureNote     string
	CreatedAt       time.Time
	AssignedAt      time.Time
	UpdatedAt       time.Time
	Items           []CourierDeliveryItem
}

type CourierDeliveryItem struct {
	ProductName string
	Quantity    int
}

package checkout

import (
	"time"

	"github.com/kyambuthia/sokomoko/internal/db"
)

type store interface {
	GetCartItems(userID int) ([]cartItem, float64, error)
	GetCartItemsForCheckout(userID int, reservationKey string) ([]cartItem, float64, error)
	GetCheckoutPlacementByIdempotency(userID int, idempotencyKey string) (*db.CheckoutPlacement, error)
	PlaceOrderFromCartWithPricingAndPayment(userID int, deliveryAddress string, totalAmount float64, deliveryNotice string, payment db.PaymentRecordInput, idempotencyKey string) (db.CheckoutPlacement, error)
	ReserveCartForCheckout(userID int, reservationKey string, expiresAt time.Time) error
}

type cartItem struct {
	ProductID     int
	ProductName   string
	UnitPrice     float64
	Quantity      int
	StockQuantity int
	LineTotal     float64
}

type dbStore struct {
	store *db.Store
}

func newDBStore(store *db.Store) store {
	return &dbStore{store: store}
}

func (s *dbStore) GetCartItems(userID int) ([]cartItem, float64, error) {
	items, subtotal, err := s.store.GetCartItems(userID)
	if err != nil {
		return nil, 0, err
	}

	mapped := make([]cartItem, 0, len(items))
	for _, item := range items {
		mapped = append(mapped, cartItem{
			ProductID:     item.ProductID,
			ProductName:   item.ProductName,
			UnitPrice:     item.UnitPrice,
			Quantity:      item.Quantity,
			StockQuantity: item.StockQuantity,
			LineTotal:     item.LineTotal,
		})
	}
	return mapped, subtotal, nil
}

func (s *dbStore) GetCheckoutPlacementByIdempotency(userID int, idempotencyKey string) (*db.CheckoutPlacement, error) {
	return s.store.GetCheckoutPlacementByIdempotency(userID, idempotencyKey)
}

func (s *dbStore) GetCartItemsForCheckout(userID int, reservationKey string) ([]cartItem, float64, error) {
	items, subtotal, err := s.store.GetCartItemsForCheckout(userID, reservationKey)
	if err != nil {
		return nil, 0, err
	}
	mapped := make([]cartItem, 0, len(items))
	for _, item := range items {
		mapped = append(mapped, cartItem{
			ProductID:     item.ProductID,
			ProductName:   item.ProductName,
			UnitPrice:     item.UnitPrice,
			Quantity:      item.Quantity,
			StockQuantity: item.StockQuantity,
			LineTotal:     item.LineTotal,
		})
	}
	return mapped, subtotal, nil
}

func (s *dbStore) PlaceOrderFromCartWithPricingAndPayment(userID int, deliveryAddress string, totalAmount float64, deliveryNotice string, payment db.PaymentRecordInput, idempotencyKey string) (db.CheckoutPlacement, error) {
	return s.store.PlaceOrderFromCartWithPricingAndPayment(userID, deliveryAddress, totalAmount, deliveryNotice, payment, idempotencyKey)
}

func (s *dbStore) ReserveCartForCheckout(userID int, reservationKey string, expiresAt time.Time) error {
	return s.store.ReserveCartForCheckout(userID, reservationKey, expiresAt)
}

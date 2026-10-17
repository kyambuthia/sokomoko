package checkout

import (
	"time"

	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/money"
)

// store is the subset of *db.Store the checkout service depends on.
type store interface {
	GetCartItemsForCheckout(userID int, reservationKey string) ([]db.CartItem, money.Cents, error)
	GetCheckoutByToken(userID int, token string) (*db.Checkout, error)
	GetLatestOpenCheckout(userID int) (*db.Checkout, error)
	GetCheckoutPlacementByIdempotency(userID int, idempotencyKey string) (*db.CheckoutPlacement, error)
	PlaceOrderFromCheckoutWithPayment(userID int, checkoutToken string, deliveryAddress string, deliveryNotice string, payment db.PaymentRecordInput) (db.CheckoutPlacement, error)
	ReserveCartForCheckout(userID int, reservationKey string, expiresAt time.Time) error
	UpdateCheckoutDraft(userID int, token string, deliveryAddress string, paymentMethod string) (*db.Checkout, error)
	UpsertCheckout(userID int, input db.CheckoutInput) (*db.Checkout, error)
}

var _ store = (*db.Store)(nil)

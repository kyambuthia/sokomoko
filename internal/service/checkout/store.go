package checkout

import (
	"context"
	"time"

	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/money"
)

// store is the subset of *db.Store the checkout service depends on.
type store interface {
	GetCommerceSettings(ctx context.Context) (db.CommerceSettings, error)
	GetCartItemsForCheckout(ctx context.Context, userID int, reservationKey string) ([]db.CartItem, money.Cents, error)
	GetCheckoutByToken(ctx context.Context, userID int, token string) (*db.Checkout, error)
	GetLatestOpenCheckout(ctx context.Context, userID int) (*db.Checkout, error)
	GetCheckoutPlacementByIdempotency(ctx context.Context, userID int, idempotencyKey string) (*db.CheckoutPlacement, error)
	PlaceOrderFromCheckoutWithPayment(ctx context.Context, userID int, checkoutToken string, deliveryAddress string, deliveryNotice string, payment db.PaymentRecordInput) (db.CheckoutPlacement, error)
	ReserveCartForCheckout(ctx context.Context, userID int, reservationKey string, expiresAt time.Time) error
	UpdateCheckoutDraft(ctx context.Context, userID int, token string, deliveryAddress string, paymentMethod string) (*db.Checkout, error)
	UpsertCheckout(ctx context.Context, userID int, input db.CheckoutInput) (*db.Checkout, error)
}

var _ store = (*db.Store)(nil)

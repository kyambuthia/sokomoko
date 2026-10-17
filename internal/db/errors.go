package db

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrUniqueConstraint          = errors.New("unique constraint violation")
	ErrTransient                 = errors.New("transient database error")
	ErrProductSlugConflict       = errors.New("product slug conflict")
	ErrDeliveryAddressRequired   = errors.New("delivery address required")
	ErrCartEmpty                 = errors.New("cart is empty")
	ErrInvalidQuantity           = errors.New("invalid quantity")
	ErrProductNotFound           = errors.New("product not found")
	ErrCheckoutNotFound          = errors.New("checkout not found")
	ErrCheckoutExpired           = errors.New("checkout expired")
	ErrInsufficientStock         = errors.New("insufficient stock")
	ErrOrderNotFound             = errors.New("order not found")
	ErrInvalidOrderState         = errors.New("invalid order state")
	ErrInvalidPartnerTransition  = errors.New("invalid partner status transition")
	ErrInvalidDeliveryTransition = errors.New("invalid delivery status transition")
	ErrInvalidPartnerStatus      = errors.New("invalid partner status")
	ErrInvalidDeliveryStatus     = errors.New("invalid delivery status")
)

const (
	pgUniqueViolation     = "23505"
	pgCheckViolation      = "23514"
	pgForeignKeyViolation = "23503"
)

func IsUniqueConstraintError(err error) bool {
	if errors.Is(err, ErrUniqueConstraint) {
		return true
	}
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation
}

func IsTransientError(err error) bool {
	if errors.Is(err, ErrTransient) {
		return true
	}
	return isTransientPgError(err)
}

func isTransientPgError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, driver.ErrBadConn) {
		return true
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch {
		case pgErr.Code == "40001", pgErr.Code == "40P01": // serialization failure, deadlock
			return true
		case pgErr.Code == "53300", pgErr.Code == "57P03": // too many connections, cannot connect now
			return true
		case len(pgErr.Code) == 5 && pgErr.Code[:2] == "08": // connection exception class
			return true
		}
		return false
	}
	return pgconn.SafeToRetry(err)
}

func constraintName(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.ConstraintName
	}
	return ""
}

func pgCode(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

// wrapDBError tags driver errors with the package's typed sentinel errors while
// keeping the original error in the chain.
func wrapDBError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case pgCode(err) == pgUniqueViolation:
		return fmt.Errorf("%w: %w", ErrUniqueConstraint, err)
	case pgCode(err) == pgCheckViolation && constraintName(err) == "inventory_stocks_not_oversold":
		return fmt.Errorf("%w: %w", ErrInsufficientStock, err)
	case isTransientPgError(err):
		return fmt.Errorf("%w: %w", ErrTransient, err)
	default:
		return err
	}
}

func wrapProductCreateError(err error) error {
	if err == nil {
		return nil
	}
	if pgCode(err) == pgUniqueViolation && constraintName(err) == "products_slug_active_key" {
		return fmt.Errorf("%w: %w", ErrProductSlugConflict, err)
	}
	return wrapDBError(err)
}

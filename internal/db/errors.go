package db

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrUniqueConstraint        = errors.New("unique constraint violation")
	ErrTransient               = errors.New("transient database error")
	ErrProductSlugConflict     = errors.New("product slug conflict")
	ErrDeliveryAddressRequired = errors.New("delivery address required")
	ErrNegativeTotalAmount     = errors.New("negative total amount")
	ErrCartEmpty               = errors.New("cart is empty")
	ErrInsufficientStock       = errors.New("insufficient stock")
)

func IsUniqueConstraintError(err error) bool {
	return errors.Is(err, ErrUniqueConstraint) || isUniqueConstraintMessage(err)
}

func IsTransientError(err error) bool {
	return errors.Is(err, ErrTransient) || isTransientMessage(err)
}

func wrapDBError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case isUniqueConstraintMessage(err):
		return fmt.Errorf("%w: %v", ErrUniqueConstraint, err)
	case isTransientMessage(err):
		return fmt.Errorf("%w: %v", ErrTransient, err)
	default:
		return err
	}
}

func wrapProductCreateError(err error) error {
	if err == nil {
		return nil
	}

	message := strings.ToLower(strings.TrimSpace(err.Error()))
	if strings.Contains(message, "unique constraint failed: products.slug") {
		return fmt.Errorf("%w: %v", ErrProductSlugConflict, err)
	}

	return wrapDBError(err)
}

func isUniqueConstraintMessage(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(msg, "unique constraint failed")
}

func isTransientMessage(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(msg, "database is locked") || strings.Contains(msg, "database is busy")
}

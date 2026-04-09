package payment

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/db"
)

var ErrInvalidMethod = errors.New("invalid payment method")

const (
	MethodCashOnDelivery    = "cash_on_delivery"
	MethodCardPlaceholder   = "card_placeholder"
	MethodMobilePlaceholder = "mobile_money_placeholder"
)

type MethodOption struct {
	Value string
	Label string
}

// Service is stateless for now. Unlike the other service packages that expose a
// local store adapter in store.go, payment works directly with db value types
// until it needs database-backed behavior of its own.
type Service struct{}

func New() *Service {
	return &Service{}
}

func (s *Service) GenerateIdempotencyKey() (string, error) {
	buf := make([]byte, 18)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func (s *Service) SupportedMethods() []string {
	return []string{
		MethodCashOnDelivery,
		MethodCardPlaceholder,
		MethodMobilePlaceholder,
	}
}

func (s *Service) SupportedMethodOptions() []MethodOption {
	methods := s.SupportedMethods()
	options := make([]MethodOption, 0, len(methods))
	for _, method := range methods {
		options = append(options, MethodOption{
			Value: method,
			Label: s.MethodLabel(method),
		})
	}
	return options
}

func (s *Service) MethodLabel(method string) string {
	switch strings.TrimSpace(strings.ToLower(method)) {
	case MethodCardPlaceholder:
		return "Card (placeholder)"
	case MethodMobilePlaceholder:
		return "Mobile Money (placeholder)"
	default:
		return "Cash on Delivery"
	}
}

func (s *Service) IsSupportedMethod(method string) bool {
	method = strings.TrimSpace(strings.ToLower(method))
	for _, candidate := range s.SupportedMethods() {
		if method == candidate {
			return true
		}
	}
	return false
}

func (s *Service) BuildRecord(method string, totalAmount float64) (db.PaymentRecordInput, error) {
	method = strings.TrimSpace(strings.ToLower(method))
	record := db.PaymentRecordInput{
		Method:   method,
		Currency: "USD",
		Amount:   totalAmount,
	}

	switch method {
	case MethodCashOnDelivery:
		record.Provider = "manual_cash_on_delivery"
		record.Status = db.PaymentStatusPending
		return record, nil
	case MethodCardPlaceholder:
		record.Provider = "placeholder_card"
		record.Status = db.PaymentStatusCaptured
	case MethodMobilePlaceholder:
		record.Provider = "placeholder_mobile_money"
		record.Status = db.PaymentStatusCaptured
	default:
		return db.PaymentRecordInput{}, ErrInvalidMethod
	}

	externalReference, err := s.GenerateIdempotencyKey()
	if err != nil {
		return db.PaymentRecordInput{}, err
	}
	record.ExternalReference = "pay_" + externalReference
	return record, nil
}

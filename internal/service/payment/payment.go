package payment

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/money"
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
	return []string{MethodCashOnDelivery}
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

func (s *Service) BuildRecord(method string, totalAmount money.Cents) (db.PaymentRecordInput, error) {
	method = strings.TrimSpace(strings.ToLower(method))
	record := db.PaymentRecordInput{
		Method:   method,
		Currency: "USD",
		Amount:   totalAmount,
	}

	// A payment is captured only after verified settlement by a real provider.
	// Historical placeholder method names are retained for displaying old orders.
	if method != MethodCashOnDelivery {
		return db.PaymentRecordInput{}, ErrInvalidMethod
	}
	record.Provider = "manual_cash_on_delivery"
	record.Status = db.PaymentStatusPending
	return record, nil
}

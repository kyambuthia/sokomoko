package payment

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/money"
	"github.com/kyambuthia/sokomoko/internal/payments"
)

// customerReason turns provider errors into text safe to show customers.
func customerReason(err error) string {
	switch {
	case errors.Is(err, payments.ErrInvalidPhone):
		return "Enter a valid M-Pesa number, for example 0712 345 678."
	case errors.Is(err, payments.ErrNotConfigured):
		return "This payment method is not set up yet."
	case errors.Is(err, money.ErrOverflow):
		return "The amount is too large for this payment method."
	case errors.Is(err, payments.ErrUnsupported):
		return "This payment method cannot charge this order."
	default:
		return "The payment could not be started. Try again."
	}
}

// CustomerReason exposes customerReason to handlers.
func CustomerReason(err error) string { return customerReason(err) }

// StatusView is what the customer sees while paying.
type StatusView struct {
	OrderID    int
	Method     string
	MethodName string
	Status     string // "pending", "paid", "failed", "review", "refunded", "cancelled", "offline"
	Message    string
	Amount     money.Cents
	Charged    money.Cents // what the provider requests, when rounded
	Currency   string
	NeedsPhone bool
	Phone      string
	Attempted  bool // a provider request has been made
}

// Status reports the payment state for the customer's order. For a pending
// attempt it asks the provider directly (throttled), so payments complete
// even when the provider cannot reach this server, as in local development.
func (s *Service) Status(ctx context.Context, userID, orderID int) (StatusView, error) {
	payment, err := s.store.GetOrderPayment(ctx, userID, orderID)
	if err != nil {
		return StatusView{}, err
	}
	view := StatusView{
		OrderID: orderID, Method: payment.Method, MethodName: s.MethodLabel(payment.Method),
		Amount: payment.Amount, Currency: payment.Currency, Attempted: payment.ExternalReference != "",
	}
	if p, ok := s.registry.Get(payment.Method); ok {
		view.NeedsPhone = p.Descriptor().NeedsPhone
	}
	// An order with no attempt yet has no payload; any other failure must not
	// pass for an empty one.
	payload, err := s.store.PaymentPayload(ctx, payment.AttemptID)
	if err != nil && !errors.Is(err, db.ErrPaymentNotFound) {
		return StatusView{}, err
	}
	view.Phone = payload["phone"]
	if charged, err := strconv.ParseInt(payload["charged_cents"], 10, 64); err == nil {
		view.Charged = money.Cents(charged)
	}

	switch {
	case !s.IsOnline(payment.Method):
		view.Status = "offline"
		return view, nil
	case payment.OrderStatus == db.OrderStatusCancelled && payment.Status == db.PaymentStatusRefunded:
		view.Status = "cancelled"
		view.Message = "The payment was refunded after this order was cancelled."
		return view, nil
	case payment.OrderStatus == db.OrderStatusCancelled && payment.Status == db.PaymentStatusCaptured:
		view.Status = "cancelled"
		view.Message = "Payment arrived after this order was cancelled. Contact the store for a refund."
		return view, nil
	case payment.ReviewRequired:
		view.Status = "review"
		view.Message = "M-Pesa reported an amount discrepancy. The transaction is under review; please wait for the store before retrying."
		return view, nil
	case payment.Status == db.PaymentStatusRefunded:
		view.Status = "refunded"
		view.Message = "This payment was refunded after review. Contact the store if you still need this order."
		return view, nil
	case payment.Status == db.PaymentStatusCaptured:
		view.Status = "paid"
		return view, nil
	case payment.OrderStatus == db.OrderStatusCancelled || payment.Status == db.PaymentStatusFailed:
		view.Status = "cancelled"
		view.Message = "This order was cancelled because payment was not completed."
		return view, nil
	}
	view.Status = "pending"
	if view.Attempted && s.dueForVerify(ctx, payment.AttemptID, payment.ExternalReference) {
		if err := s.verifyAndApply(ctx, payment, payload); err != nil {
			slog.Warn("payment verification failed", "payment_id", payment.ID, "error", err)
		}
		refreshed, err := s.store.GetOrderPayment(ctx, userID, orderID)
		if err != nil {
			slog.Warn("payment status refresh failed", "payment_id", payment.ID, "error", err)
		} else if refreshed.Status == db.PaymentStatusCaptured {
			view.Status = "paid"
			return view, nil
		}
	}
	reason, err := s.store.LastPaymentError(ctx, payment.ID)
	if err != nil {
		slog.Warn("payment attempt error lookup failed", "payment_id", payment.ID, "error", err)
	} else if reason != "" {
		view.Status = "failed"
		view.Message = reason
	}
	return view, nil
}

// dueForVerify coordinates status-page provider checks across replicas when
// the store supports it, falling back to a bounded process-local throttle.
func (s *Service) dueForVerify(ctx context.Context, attemptID int, reference string) bool {
	if throttle, ok := s.store.(paymentVerificationThrottle); ok {
		allowed, err := throttle.TryAcquirePaymentVerification(ctx, attemptID, verifyInterval)
		if err == nil {
			s.rememberVerificationLocal(reference, time.Now())
			return allowed
		}
		slog.WarnContext(ctx, "shared payment verification throttle unavailable; using process-local fallback", "attempt_id", attemptID, "error", err)
	}
	return s.dueForVerifyLocal(reference)
}

// rememberVerificationLocal warms the fallback after a healthy shared-throttle
// result, so a following database error cannot immediately repeat the check.
func (s *Service) rememberVerificationLocal(reference string, now time.Time) {
	key := hashToken(reference)
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.verified == nil {
		s.verified = make(map[string]time.Time)
	}
	if _, exists := s.verified[key]; exists || len(s.verified) < maxVerifiedReferences {
		s.verified[key] = now
		return
	}
	s.saturatedUntil = now.Add(verifyInterval)
}

// dueForVerifyLocal throttles provider checks per attempt in this process. A
// retry has a new reference, so it is checked straight away.
func (s *Service) dueForVerifyLocal(reference string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	key := hashToken(reference)
	if s.verified == nil {
		s.verified = make(map[string]time.Time)
	}
	if last, ok := s.verified[key]; ok && now.Sub(last) < verifyInterval {
		return false
	}
	if _, exists := s.verified[key]; exists {
		s.verified[key] = now
		return true
	}
	if len(s.verified) < maxVerifiedReferences {
		s.verified[key] = now
		return true
	}
	if s.saturatedUntil.After(now) {
		return false
	}

	var earliestExpiry time.Time
	for candidate, last := range s.verified {
		expiry := last.Add(verifyInterval)
		if !expiry.After(now) {
			delete(s.verified, candidate)
			continue
		}
		if earliestExpiry.IsZero() || expiry.Before(earliestExpiry) {
			earliestExpiry = expiry
		}
	}
	if len(s.verified) < maxVerifiedReferences {
		s.saturatedUntil = time.Time{}
		s.verified[key] = now
		return true
	}
	s.saturatedUntil = earliestExpiry
	return false
}

// verifyAndApply asks the provider for the result and records it.
func (s *Service) verify(ctx context.Context, payment *db.OnlinePayment, payload map[string]string) (payments.Verification, error) {
	provider, cfg, err := s.providerFor(ctx, payment.Provider)
	if err != nil {
		return payments.Verification{}, err
	}
	return s.verifyWith(ctx, provider, cfg, payment, payload)
}

func (s *Service) verifyWith(ctx context.Context, provider payments.Provider, cfg payments.Config, payment *db.OnlinePayment, payload map[string]string) (payments.Verification, error) {
	if payload["callback_amount_mismatch"] == "true" {
		return payments.Verification{}, db.ErrPaymentAmountMatch
	}
	charged := payment.Amount
	if c, err := strconv.ParseInt(payload["charged_cents"], 10, 64); err == nil && c > 0 {
		charged = money.Cents(c)
	}
	return provider.Verify(ctx, cfg, payments.VerifyRequest{
		Reference: payment.ExternalReference, Data: payload, Charged: charged, Currency: payment.Currency,
	})
}

func (s *Service) verifyAndApply(ctx context.Context, payment *db.OnlinePayment, payload map[string]string) error {
	v, err := s.verify(ctx, payment, payload)
	if err != nil {
		return err
	}
	return s.apply(ctx, payment, v)
}

func (s *Service) apply(ctx context.Context, payment *db.OnlinePayment, v payments.Verification) error {
	switch v.Status {
	case payments.StatusPaid:
		currency := v.Currency
		if currency == "" {
			currency = payment.Currency
		}
		captured, err := s.store.CapturePayment(ctx, payment.AttemptID, v.Amount, currency, v.Receipt)
		if err != nil {
			return err
		}
		if payment.Status == db.PaymentStatusCaptured && payment.AttemptStatus != db.PaymentAttemptStatusCompleted {
			slog.Warn("duplicate payment captured; refund required", "payment_id", payment.ID, "attempt_id", payment.AttemptID)
		}
		if captured {
			slog.Info("payment captured", "payment_id", payment.ID, "order_id", payment.OrderID, "provider", payment.Provider)
			if payment.OrderStatus == db.OrderStatusCancelled {
				slog.Warn("payment captured after order cancellation; refund required", "payment_id", payment.ID, "order_id", payment.OrderID)
			}
		}
	case payments.StatusFailed:
		return s.store.FailPaymentAttempt(ctx, payment.AttemptID, v.Reason)
	}
	return nil
}

// HandleReturn confirms a payment when the customer's browser comes back
// from a hosted payment page. It returns the order to show.
func (s *Service) HandleReturn(ctx context.Context, userID int, reference string) (int, error) {
	payment, err := s.findByReference(ctx, reference)
	if err != nil {
		return 0, err
	}
	if payment.UserID != userID {
		return 0, db.ErrPaymentNotFound
	}
	payload, err := s.store.PaymentPayload(ctx, payment.AttemptID)
	if err != nil {
		return payment.OrderID, err
	}
	if payment.AttemptStatus != db.PaymentAttemptStatusCompleted {
		if err := s.verifyAndApply(ctx, payment, payload); err != nil {
			return payment.OrderID, err
		}
	}
	return payment.OrderID, nil
}

func (s *Service) findByReference(ctx context.Context, reference string) (*db.OnlinePayment, error) {
	reference = strings.TrimSpace(reference)
	for _, p := range s.registry.All() {
		if !p.Descriptor().Online {
			continue
		}
		payment, err := s.store.GetPaymentByReference(ctx, p.Descriptor().ID, reference)
		if errors.Is(err, db.ErrPaymentNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if payment != nil {
			return payment, nil
		}
	}
	return nil, db.ErrPaymentNotFound
}

// HandleWebhook processes a signed provider notification (Paystack).
func (s *Service) HandleWebhook(ctx context.Context, providerID string, r *http.Request, body []byte) error {
	provider, cfg, err := s.providerFor(ctx, providerID)
	if err != nil {
		return err
	}
	parser, ok := provider.(payments.WebhookParser)
	if !ok {
		return payments.ErrUnsupported
	}
	event, err := parser.ParseWebhook(cfg, r, body)
	if err != nil {
		return err
	}
	payment, err := s.store.GetPaymentByReference(ctx, providerID, event.Reference)
	if err != nil {
		return err
	}
	if payment.AttemptStatus == db.PaymentAttemptStatusCompleted {
		return nil
	}
	payload, err := s.store.PaymentPayload(ctx, payment.AttemptID)
	if err != nil {
		return err
	}
	verified, err := s.verify(ctx, payment, payload)
	if err != nil {
		return err
	}
	if verified.Status == payments.StatusPending {
		return ErrProviderVerificationPending
	}
	return s.apply(ctx, payment, verified)
}

// HandleCallback processes an unsigned provider callback (M-Pesa). The
// per-payment token in the URL authenticates it, the provider identifiers
// must match the stored attempt, and the callback outcome is confirmed with
// the provider before it changes payment state.
func (s *Service) HandleCallback(ctx context.Context, providerID, token string, r *http.Request, body []byte) error {
	provider, cfg, err := s.providerFor(ctx, providerID)
	if err != nil {
		return err
	}
	parser, ok := provider.(payments.WebhookParser)
	if !ok {
		return payments.ErrUnsupported
	}
	payment, err := s.store.GetPaymentByCallbackToken(ctx, providerID, hashToken(token))
	if err != nil {
		return err
	}
	event, err := parser.ParseWebhook(cfg, r, body)
	if err != nil {
		return err
	}
	payload, err := s.store.PaymentPayload(ctx, payment.AttemptID)
	if err != nil {
		return err
	}
	if id := event.Data["checkout_request_id"]; id == "" || payload["checkout_request_id"] != "" && id != payload["checkout_request_id"] {
		return fmt.Errorf("%w: callback does not match the stored request", db.ErrPaymentNotFound)
	}
	if payload["checkout_request_id"] == "" {
		payload["checkout_request_id"] = event.Data["checkout_request_id"]
		if err := s.store.UpdatePaymentPayload(ctx, payment.AttemptID, map[string]string{"checkout_request_id": payload["checkout_request_id"]}); err != nil {
			return err
		}
	}
	if payload["callback_amount_mismatch"] == "true" {
		return nil
	}
	if event.Result == nil {
		return nil
	}
	if event.Result.Status == payments.StatusFailed && payment.AttemptStatus == db.PaymentAttemptStatusCompleted {
		return nil
	}
	confirmed, err := s.verifyWith(ctx, provider, cfg, payment, payload)
	if err != nil {
		return err
	}
	if confirmed.Status == payments.StatusPending {
		return ErrProviderVerificationPending
	}
	if confirmed.Status != payments.StatusPaid {
		return s.apply(ctx, payment, confirmed)
	}
	if event.Result.Status == payments.StatusPaid {
		reportedCurrency := event.Result.Currency
		if reportedCurrency == "" {
			reportedCurrency = payment.Currency
		}
		if event.Result.Amount <= 0 || event.Result.Amount != confirmed.Amount || !strings.EqualFold(reportedCurrency, confirmed.Currency) {
			recorder, ok := s.store.(paymentAmountMismatchRecorder)
			if !ok {
				return db.ErrPaymentAmountMatch
			}
			if err := recorder.RecordPaymentAmountMismatch(ctx, payment.AttemptID, event.Result.Amount, reportedCurrency, event.Result.Receipt); err != nil {
				return err
			}
			slog.ErrorContext(ctx, "M-Pesa callback amount requires review", "payment_id", payment.ID, "attempt_id", payment.AttemptID,
				"requested_cents", confirmed.Amount, "reported_cents", event.Result.Amount, "reported_currency", reportedCurrency)
			return nil
		}
		if event.Result.Receipt != "" {
			confirmed.Receipt = event.Result.Receipt
		}
	}
	return s.apply(ctx, payment, confirmed)
}

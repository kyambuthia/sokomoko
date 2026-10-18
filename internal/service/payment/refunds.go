package payment

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/payments"
)

var (
	ErrRefundUnsupported       = errors.New("automatic refunds are unavailable for this provider")
	ErrRefundReferenceRequired = errors.New("enter the M-Pesa reversal receipt")
	ErrRefundNotReady          = errors.New("refund has no provider reference to check")
	ErrRefundNotResolvable     = errors.New("only a refund under review or awaiting the provider can be closed by hand")
)

func (s *Service) PaymentExceptions() ([]db.PaymentException, error) {
	return s.store.ListPaymentExceptions(50)
}

// RequestRefund either initiates a Paystack refund once or records an M-Pesa
// reversal already completed in the operator's M-Pesa account. A claimed
// request is never sent again after an uncertain provider response.
func (s *Service) RequestRefund(ctx context.Context, actor *Actor, id int, manualReference string) (*db.PaymentException, error) {
	if actor == nil || actor.Role != db.RoleAdmin {
		return nil, ErrForbidden
	}
	e, err := s.store.GetPaymentException(id)
	if err != nil {
		return nil, err
	}
	var refundProvider payments.RefundProvider
	var cfg payments.Config
	switch e.Provider {
	case "mpesa":
		manualReference = strings.TrimSpace(manualReference)
		if manualReference == "" {
			return nil, ErrRefundReferenceRequired
		}
	case "paystack":
		provider, config, err := s.providerFor(e.Provider)
		if err != nil {
			return nil, err
		}
		var ok bool
		refundProvider, ok = provider.(payments.RefundProvider)
		if !ok {
			return nil, ErrRefundUnsupported
		}
		cfg = config
	default:
		return nil, ErrRefundUnsupported
	}
	e, claimed, err := s.store.BeginRefund(id)
	if err != nil || !claimed {
		return e, err
	}
	if e.Provider == "mpesa" {
		if err := s.store.SetPaymentExceptionStatus(id, "refunded", manualReference, ""); err != nil {
			return nil, err
		}
		_ = s.store.CreateAuditLog(actor.ID, "payments.refund.manual", "payment_exception", id, "M-Pesa reversal recorded")
		return s.store.GetPaymentException(id)
	}
	result, err := refundProvider.CreateRefund(ctx, cfg, e.RequestReference, e.Amount, e.Currency)
	if err != nil {
		_ = s.store.SetPaymentExceptionStatus(id, "review", "", "Refund request outcome unknown; reconcile in Paystack before retrying.")
		slog.Error("refund request requires reconciliation", "exception_id", id, "error", err)
		return nil, err
	}
	if err := s.recordRefundResult(e, result); err != nil {
		return nil, err
	}
	_ = s.store.CreateAuditLog(actor.ID, "payments.refund.request", "payment_exception", id, "Paystack refund initiated")
	return s.store.GetPaymentException(id)
}

// ResolveRefund closes an exception the operator reconciled in the provider
// dashboard after an uncertain or failed automatic refund. Open exceptions go
// through RequestRefund so a Paystack refund is never skipped by mistake.
func (s *Service) ResolveRefund(actor *Actor, id int, reference string) (*db.PaymentException, error) {
	if actor == nil || actor.Role != db.RoleAdmin {
		return nil, ErrForbidden
	}
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return nil, ErrRefundReferenceRequired
	}
	e, err := s.store.GetPaymentException(id)
	if err != nil {
		return nil, err
	}
	switch e.Status {
	case "refunded":
		return e, nil
	case "requested", "pending", "review":
	default:
		return nil, ErrRefundNotResolvable
	}
	if err := s.store.SetPaymentExceptionStatus(id, "refunded", reference, ""); err != nil {
		return nil, err
	}
	_ = s.store.CreateAuditLog(actor.ID, "payments.refund.resolve", "payment_exception", id, "Refund reconciled by hand: "+truncate(reference, 100))
	return s.store.GetPaymentException(id)
}

// ExceptionsNeedingAttention counts exceptions an admin must act on.
func (s *Service) ExceptionsNeedingAttention(ctx context.Context) (int, error) {
	open, _, review, err := s.store.PaymentExceptionCounts(ctx)
	return open + review, err
}

func truncate(value string, n int) string {
	if len(value) <= n {
		return value
	}
	return value[:n]
}

func (s *Service) RefreshRefund(ctx context.Context, actor *Actor, id int) (*db.PaymentException, error) {
	if actor == nil || actor.Role != db.RoleAdmin {
		return nil, ErrForbidden
	}
	e, err := s.store.GetPaymentException(id)
	if err != nil {
		return nil, err
	}
	if e.Provider != "paystack" {
		return nil, ErrRefundUnsupported
	}
	if e.RefundReference == "" {
		return nil, ErrRefundNotReady
	}
	if e.Status == "refunded" {
		return e, nil
	}
	provider, cfg, err := s.providerFor(e.Provider)
	if err != nil {
		return nil, err
	}
	refunder, ok := provider.(payments.RefundProvider)
	if !ok {
		return nil, ErrRefundUnsupported
	}
	result, err := refunder.FetchRefund(ctx, cfg, e.RefundReference)
	if err != nil {
		return nil, err
	}
	if result.ID != e.RefundReference {
		return nil, fmt.Errorf("%w: refund reference mismatch", payments.ErrProviderFailure)
	}
	if err := s.recordRefundResult(e, result); err != nil {
		return nil, err
	}
	return s.store.GetPaymentException(id)
}

func (s *Service) recordRefundResult(e *db.PaymentException, result payments.RefundResult) error {
	if result.Amount != e.Amount || !strings.EqualFold(result.Currency, e.Currency) {
		return fmt.Errorf("%w: refund amount or currency mismatch", payments.ErrProviderFailure)
	}
	status, detail := "review", "Check the refund in the provider dashboard."
	switch result.Status {
	case "processed":
		status, detail = "refunded", ""
	case "pending", "processing":
		status, detail = "pending", ""
	case "needs-attention":
		status, detail = "review", "Provider needs customer bank details."
	case "failed":
		status, detail = "review", "Provider reports a failed refund."
	}
	return s.store.SetPaymentExceptionStatus(e.ID, status, result.ID, detail)
}

package payment

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/domain"
	"github.com/kyambuthia/sokomoko/internal/payments"
)

// ErrReversalAmountMismatch is returned when a provider reversal response does
// not confirm the expected amount. The exception stays on the manual queue.
var ErrReversalAmountMismatch = payments.ErrReversalAmountMismatch

var (
	ErrRefundUnsupported       = errors.New("automatic refunds are unavailable for this provider")
	ErrRefundReferenceRequired = errors.New("enter the M-Pesa reversal receipt")
	ErrRefundNotReady          = errors.New("refund has no provider reference to check")
	ErrRefundNotResolvable     = errors.New("only a reconciled refund can be closed by hand")
	ErrRefundRequestInProgress = errors.New("the provider refund result is still being reconciled; try again shortly")
)

func (s *Service) PaymentExceptions(ctx context.Context) ([]db.PaymentException, error) {
	return s.store.ListPaymentExceptions(ctx, 50)
}

// RequestRefund initiates a Paystack refund once, or an M-Pesa reversal when
// Daraja confirms the expected amount. A Paystack request is claimed before
// the provider call and is never sent again after an uncertain response. An
// M-Pesa reversal stays on the manual queue unless the response amount matches.
func (s *Service) RequestRefund(ctx context.Context, actor *Actor, id int, manualReference string) (*db.PaymentException, error) {
	if actor == nil || actor.Role != domain.RoleAdmin {
		return nil, ErrForbidden
	}
	e, err := s.store.GetPaymentException(ctx, id)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, db.ErrPaymentExceptionNotFound
	}
	var refundProvider payments.RefundProvider
	var cfg payments.Config
	switch e.Provider {
	case "mpesa":
		return s.requestMpesaReversal(ctx, actor, e, manualReference)
	case "paystack":
		provider, config, err := s.providerFor(ctx, e.Provider)
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
	e, claimed, err := s.store.BeginRefundForActor(ctx, actor.ID, id)
	if errors.Is(err, db.ErrWorkspaceActorNotFound) {
		return nil, ErrForbidden
	}
	if err != nil || !claimed {
		return e, err
	}
	// The refund is claimed and is never sent twice, so finish recording it
	// even if the admin's browser goes away. Provider calls keep their own
	// timeout.
	ctx = context.WithoutCancel(ctx)
	result, err := refundProvider.CreateRefund(ctx, cfg, e.RequestReference, e.Amount, e.Currency)
	if err != nil {
		if markErr := s.store.SetPaymentExceptionStatus(ctx, id, "review", "", "Refund request outcome unknown; reconcile in Paystack before retrying."); markErr != nil {
			slog.Error("mark refund for review", "exception_id", id, "error", markErr)
		}
		slog.Error("refund request requires reconciliation", "exception_id", id, "error", err)
		return nil, err
	}
	if err := s.recordRefundResult(ctx, e, result); err != nil {
		return nil, err
	}
	s.logAudit(ctx, actor.ID, "payments.refund.request", "payment_exception", id, "Paystack refund initiated")
	return s.store.GetPaymentException(ctx, id)
}

// requestMpesaReversal calls Daraja before the exception leaves the manual
// queue. A missing or different amount, or any provider error, leaves the
// row open so an operator can reconcile it by hand.
func (s *Service) requestMpesaReversal(ctx context.Context, actor *Actor, e *db.PaymentException, manualReference string) (*db.PaymentException, error) {
	manualReference = strings.TrimSpace(manualReference)
	if manualReference == "" {
		return nil, ErrRefundReferenceRequired
	}
	if e.Status == "refunded" || e.Status != "open" {
		return e, nil
	}
	provider, config, err := s.providerFor(ctx, e.Provider)
	if err != nil {
		return nil, err
	}
	refunder, ok := provider.(payments.RefundProvider)
	if !ok {
		return nil, ErrRefundUnsupported
	}
	ctx = context.WithoutCancel(ctx)
	result, err := refunder.CreateRefund(ctx, config, manualReference, e.Amount, e.Currency)
	if err != nil {
		return nil, err
	}
	e, claimed, err := s.store.BeginRefundForActor(ctx, actor.ID, e.ID)
	if errors.Is(err, db.ErrWorkspaceActorNotFound) {
		return nil, ErrForbidden
	}
	if err != nil || !claimed {
		return e, err
	}
	if err := s.recordRefundResult(ctx, e, result); err != nil {
		return nil, err
	}
	s.logAudit(ctx, actor.ID, "payments.refund.request", "payment_exception", e.ID, "M-Pesa reversal requested")
	return s.store.GetPaymentException(ctx, e.ID)
}

// ResolveRefund closes an exception the operator reconciled in the provider
// dashboard after an uncertain or failed automatic refund. Open exceptions go
// through RequestRefund so a Paystack refund is never skipped by mistake.
func (s *Service) ResolveRefund(ctx context.Context, actor *Actor, id int, reference string) (*db.PaymentException, error) {
	if actor == nil || actor.Role != domain.RoleAdmin {
		return nil, ErrForbidden
	}
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return nil, ErrRefundReferenceRequired
	}
	e, err := s.store.GetPaymentException(ctx, id)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, db.ErrPaymentExceptionNotFound
	}
	switch e.Status {
	case "refunded":
		return e, nil
	case "requested", "pending", "review":
	default:
		return nil, ErrRefundNotResolvable
	}
	if err := s.store.SetPaymentExceptionStatusForActor(ctx, actor.ID, id, "refunded", reference, ""); err != nil {
		if errors.Is(err, db.ErrWorkspaceActorNotFound) {
			return nil, ErrForbidden
		}
		if errors.Is(err, db.ErrPaymentRefundRequestInProgress) {
			return nil, ErrRefundRequestInProgress
		}
		return nil, err
	}
	s.logAudit(ctx, actor.ID, "payments.refund.resolve", "payment_exception", id, "Refund reconciled by hand: "+truncate(reference, 100))
	return s.store.GetPaymentException(ctx, id)
}

// ExceptionsNeedingAttention counts exceptions an admin must act on.
func (s *Service) ExceptionsNeedingAttention(ctx context.Context) (int, error) {
	open, _, review, err := s.store.PaymentExceptionCounts(ctx)
	return open + review, err
}

func truncate(value string, n int) string {
	runes := []rune(value)
	if len(runes) <= n {
		return value
	}
	return string(runes[:n])
}

func (s *Service) RefreshRefund(ctx context.Context, actor *Actor, id int) (*db.PaymentException, error) {
	if actor == nil || actor.Role != domain.RoleAdmin {
		return nil, ErrForbidden
	}
	e, err := s.store.GetPaymentException(ctx, id)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, db.ErrPaymentExceptionNotFound
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
	provider, cfg, err := s.providerFor(ctx, e.Provider)
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
	if err := s.recordRefundResult(ctx, e, result); err != nil {
		return nil, err
	}
	return s.store.GetPaymentException(ctx, id)
}

func (s *Service) recordRefundResult(ctx context.Context, e *db.PaymentException, result payments.RefundResult) error {
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
	return s.store.SetPaymentExceptionStatus(ctx, e.ID, status, result.ID, detail)
}

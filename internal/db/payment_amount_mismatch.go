package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/money"
)

const (
	paymentAmountMismatchMessage = "M-Pesa reported a different charge amount; review the transaction before retrying."
	paymentAmountUnknownMessage  = "M-Pesa confirmed a charge without an amount; review the transaction before retrying."
)

// RecordPaymentAmountMismatch stops an M-Pesa attempt whose verified callback
// reports an unexpected or missing amount and queues it for operator review.
// The callback token and provider verification are checked by the service.
func (s *Store) RecordPaymentAmountMismatch(ctx context.Context, attemptID int, reported money.Cents, currency, receipt string) error {
	if attemptID <= 0 {
		return ErrPaymentNotFound
	}
	currency = strings.ToUpper(strings.TrimSpace(currency))
	marker, err := json.Marshal(map[string]string{
		"callback_amount_mismatch": "true",
		"callback_amount_cents":    strconv.FormatInt(int64(reported), 10),
		"callback_amount_currency": currency,
	})
	if err != nil {
		return err
	}
	ctx, cancel := s.queryContext(ctx)
	defer cancel()

	return s.withTx(ctx, func(tx *sql.Tx) error {
		var paymentID int
		var paymentCurrency, provider string
		var paymentAmount int64
		if err := tx.QueryRowContext(ctx, `
			SELECT p.id,p.currency,p.provider,p.amount_cents
			FROM payments p JOIN payment_attempts a ON a.payment_id=p.id
			WHERE a.id=$1 FOR UPDATE OF p`, attemptID,
		).Scan(&paymentID, &paymentCurrency, &provider, &paymentAmount); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrPaymentNotFound
			}
			return err
		}
		if provider != "mpesa" {
			return ErrPaymentNotFound
		}

		var requestReference string
		var payload []byte
		if err := tx.QueryRowContext(ctx, `
			SELECT request_reference,provider_payload
			FROM payment_attempts WHERE id=$1 AND payment_id=$2 FOR UPDATE`, attemptID, paymentID,
		).Scan(&requestReference, &payload); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrPaymentNotFound
			}
			return err
		}

		data := map[string]string{}
		if err := json.Unmarshal(payload, &data); err != nil {
			return err
		}
		expected := money.Cents(paymentAmount)
		if charged, err := strconv.ParseInt(data["charged_cents"], 10, 64); err == nil && charged > 0 {
			expected = money.Cents(charged)
		}
		if expected <= 0 {
			return ErrPaymentAmountMatch
		}

		reason := "amount_mismatch"
		exceptionAmount, exceptionCurrency := reported, currency
		message := paymentAmountMismatchMessage
		if reported <= 0 {
			reason = "amount_unverified"
			exceptionAmount, exceptionCurrency = expected, strings.ToUpper(strings.TrimSpace(paymentCurrency))
			message = paymentAmountUnknownMessage
		} else if reported == expected && strings.EqualFold(paymentCurrency, currency) {
			return ErrPaymentAmountMatch
		}
		if exceptionAmount <= 0 || len(exceptionCurrency) != 3 {
			return ErrPaymentAmountMatch
		}

		if _, err := tx.ExecContext(ctx, `
			UPDATE payment_attempts SET
			    status=CASE WHEN status='completed' THEN status ELSE 'failed' END,
			    error_message=CASE WHEN status='completed' THEN error_message ELSE $2 END,
			    external_reference=CASE
			        WHEN NULLIF($3,'') IS NULL THEN external_reference
			        WHEN external_reference IS NULL OR external_reference='' OR external_reference=request_reference THEN $3
			        ELSE external_reference END,
			    provider_payload=provider_payload || $4::jsonb
			WHERE id=$1`,
			attemptID, message, strings.TrimSpace(receipt), marker,
		); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO payment_exceptions
			    (payment_id,attempt_id,reason,provider,currency,amount_cents,request_reference,receipt)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
			ON CONFLICT (attempt_id) DO NOTHING`,
			paymentID, attemptID, reason, provider, exceptionCurrency, int64(exceptionAmount), requestReference, strings.TrimSpace(receipt),
		)
		if err != nil {
			return fmt.Errorf("record M-Pesa amount exception: %w", err)
		}
		return nil
	})
}

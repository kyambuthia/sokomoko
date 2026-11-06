ALTER TABLE payment_exceptions
    ADD COLUMN affects_payment BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX payment_exceptions_unresolved_payment_idx
    ON payment_exceptions (payment_id)
    WHERE status <> 'refunded';

UPDATE payment_exceptions e
SET affects_payment = TRUE
FROM payments p, payment_attempts a
WHERE e.payment_id = p.id
  AND e.attempt_id = a.id
  AND e.reason IN ('amount_mismatch', 'amount_unverified')
  AND p.status = 'captured'
  AND p.external_reference = a.request_reference;

UPDATE payments p
SET status = 'refunded'
FROM payment_exceptions e
WHERE e.payment_id = p.id
  AND e.affects_payment
  AND e.status = 'refunded'
  AND p.status = 'captured';

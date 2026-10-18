-- Keep each provider request addressable after a retry. A delayed callback or
-- webhook must resolve to the attempt that actually initiated the charge.
ALTER TABLE payment_attempts ADD COLUMN callback_token_hash TEXT;
ALTER TABLE payment_attempts ADD COLUMN provider_payload JSONB NOT NULL DEFAULT '{}'::jsonb;

UPDATE payment_attempts a
SET callback_token_hash = p.callback_token_hash,
    provider_payload = p.provider_payload
FROM payments p
WHERE a.payment_id = p.id
  AND a.id = (SELECT max(latest.id) FROM payment_attempts latest WHERE latest.payment_id = p.id)
  AND a.request_reference = p.external_reference;

-- Older versions could create concurrent pending attempts. Retain only the
-- newest as active before enforcing the invariant for future starts.
UPDATE payment_attempts a
SET status = 'failed', error_message = 'superseded by a newer attempt'
WHERE a.status = 'pending'
  AND EXISTS (
      SELECT 1 FROM payment_attempts newer
      WHERE newer.payment_id = a.payment_id AND newer.status = 'pending' AND newer.id > a.id
  );

CREATE UNIQUE INDEX payment_attempts_one_pending_idx ON payment_attempts (payment_id) WHERE status = 'pending';
CREATE UNIQUE INDEX payment_attempts_reference_idx ON payment_attempts (request_reference) WHERE request_reference <> '';
CREATE UNIQUE INDEX payment_attempts_callback_token_idx ON payment_attempts (callback_token_hash) WHERE callback_token_hash IS NOT NULL;

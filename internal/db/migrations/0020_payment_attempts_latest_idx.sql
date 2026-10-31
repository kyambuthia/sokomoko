-- Support latest-attempt lookups while retaining the payment_id lookup prefix.
DROP INDEX payment_attempts_payment_id_idx;
CREATE INDEX payment_attempts_latest_idx ON payment_attempts (payment_id, id DESC);

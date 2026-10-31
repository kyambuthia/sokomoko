-- Coordinate provider status checks across application replicas.
ALTER TABLE payment_attempts ADD COLUMN last_verification_at TIMESTAMPTZ;

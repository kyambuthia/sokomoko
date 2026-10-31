-- Persist authentication abuse state across app replicas and database recovery.
-- The table is intentionally logged: active lockouts should survive failover.
CREATE TABLE auth_abuse_limits (
    key_hash BYTEA PRIMARY KEY CHECK (octet_length(key_hash)=32),
    success_generation BIGINT NOT NULL CHECK (success_generation >= 0),
    failure_count BIGINT NOT NULL CHECK (failure_count >= 0),
    locked_until TIMESTAMPTZ,
    expires_at TIMESTAMPTZ NOT NULL
) WITH (fillfactor=80);

CREATE INDEX auth_abuse_limits_expiry_idx ON auth_abuse_limits(expires_at);

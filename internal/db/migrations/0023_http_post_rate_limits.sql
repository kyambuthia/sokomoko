-- Shared per-IP POST counters for multi-replica deployments.
-- UNLOGGED is intentional: after a database crash the limits reset for at most
-- one configured window rather than adding write-ahead log traffic per request.
CREATE UNLOGGED TABLE http_post_rate_limits (
    key_hash BYTEA PRIMARY KEY CHECK (octet_length(key_hash)=32),
    hits BIGINT NOT NULL CHECK (hits > 0),
    reset_at TIMESTAMPTZ NOT NULL
) WITH (fillfactor=80);

CREATE INDEX http_post_rate_limits_reset_idx ON http_post_rate_limits(reset_at);

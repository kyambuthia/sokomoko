-- Keep pending reset mail durable without persisting bearer tokens in plaintext.
CREATE TABLE password_reset_email_outbox (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id        BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    sealed_payload BYTEA       NOT NULL CHECK (octet_length(sealed_payload) > 0),
    expires_at     TIMESTAMPTZ NOT NULL,
    attempts       INTEGER     NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    available_at   TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    lease_until    TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX password_reset_email_outbox_claim_idx
    ON password_reset_email_outbox (available_at, id)
    INCLUDE (lease_until, expires_at);

-- A confirmed charge can arrive after cancellation or after another attempt
-- already captured the order. Keep each excess charge visible until resolved.
CREATE TABLE payment_exceptions (
    id                  BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    payment_id          BIGINT NOT NULL REFERENCES payments (id) ON DELETE RESTRICT,
    attempt_id          BIGINT NOT NULL UNIQUE REFERENCES payment_attempts (id) ON DELETE RESTRICT,
    reason              TEXT NOT NULL CHECK (reason IN ('late_capture', 'duplicate_charge')),
    status              TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'requested', 'pending', 'review', 'refunded')),
    provider             TEXT NOT NULL,
    currency             CHAR(3) NOT NULL,
    amount_cents         BIGINT NOT NULL CHECK (amount_cents > 0),
    request_reference    TEXT NOT NULL,
    receipt              TEXT NOT NULL DEFAULT '',
    refund_reference     TEXT NOT NULL DEFAULT '',
    last_error           TEXT NOT NULL DEFAULT '',
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX payment_exceptions_open_idx ON payment_exceptions (created_at DESC)
    WHERE status <> 'refunded';
CREATE TRIGGER payment_exceptions_set_updated_at BEFORE UPDATE ON payment_exceptions
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

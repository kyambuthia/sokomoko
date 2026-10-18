-- Store currency, pricing rules, and pluggable payment providers.

-- One row of marketplace-wide money settings. Amounts are minor units of
-- currency. A free_shipping_threshold_cents of 0 disables free shipping.
CREATE TABLE commerce_settings (
    id                            SMALLINT    PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    currency                      CHAR(3)     NOT NULL DEFAULT 'USD' CHECK (currency ~ '^[A-Z]{3}$'),
    shipping_flat_cents           BIGINT      NOT NULL DEFAULT 650 CHECK (shipping_flat_cents >= 0),
    free_shipping_threshold_cents BIGINT      NOT NULL DEFAULT 8000 CHECK (free_shipping_threshold_cents >= 0),
    tax_rate_bp                   INTEGER     NOT NULL DEFAULT 800 CHECK (tax_rate_bp BETWEEN 0 AND 10000),
    updated_at                    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TRIGGER commerce_settings_set_updated_at BEFORE UPDATE ON commerce_settings
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
INSERT INTO commerce_settings (id) VALUES (1);

-- Payment provider configuration. settings holds non-secret fields as JSON;
-- secrets holds AES-GCM sealed JSON of the secret fields (see internal/secret).
CREATE TABLE payment_providers (
    provider   TEXT        PRIMARY KEY CHECK (provider ~ '^[a-z0-9_]+$'),
    enabled    BOOLEAN     NOT NULL DEFAULT false,
    mode       TEXT        NOT NULL DEFAULT 'test' CHECK (mode IN ('test', 'live')),
    settings   JSONB       NOT NULL DEFAULT '{}'::jsonb,
    secrets    BYTEA,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TRIGGER payment_providers_set_updated_at BEFORE UPDATE ON payment_providers
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
-- Keep the existing behaviour: cash on delivery stays available.
INSERT INTO payment_providers (provider, enabled, mode) VALUES ('cash_on_delivery', true, 'live');

-- Online payments: when the provider confirmed the money, and a per-payment
-- secret used to authenticate unsigned provider callbacks (stored hashed).
ALTER TABLE payments ADD COLUMN captured_at TIMESTAMPTZ;
ALTER TABLE payments ADD COLUMN callback_token_hash TEXT;
ALTER TABLE payments ADD COLUMN provider_payload JSONB NOT NULL DEFAULT '{}'::jsonb;
CREATE INDEX payments_pending_online_idx ON payments (created_at)
    WHERE status = 'pending' AND method <> 'cash_on_delivery';
CREATE INDEX payments_order_status_idx ON payments (order_id, status);

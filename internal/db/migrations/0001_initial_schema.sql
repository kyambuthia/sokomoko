-- Sokomoko PostgreSQL schema, version 1.
--
-- Conventions:
--   * Surrogate keys are BIGINT identity columns.
--   * All timestamps are TIMESTAMPTZ and default to now().
--   * Money is stored as BIGINT minor units (cents) in *_cents columns.
--   * updated_at columns are maintained by the set_updated_at() trigger.
--   * Rows that carry financial or audit history are never cascade-deleted
--     from their owning user; users are soft-deleted via deleted_at.

CREATE FUNCTION set_updated_at() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    NEW.updated_at := now();
    RETURN NEW;
END;
$$;

-- ---------------------------------------------------------------------------
-- Identity and access
-- ---------------------------------------------------------------------------

CREATE TABLE users (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    username      TEXT        NOT NULL CHECK (char_length(username) BETWEEN 3 AND 32),
    email         TEXT        NOT NULL CHECK (char_length(email) BETWEEN 3 AND 254),
    password_hash TEXT        NOT NULL,
    role          TEXT        NOT NULL DEFAULT 'user' CHECK (role IN ('user', 'staff', 'admin')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at    TIMESTAMPTZ
);
CREATE UNIQUE INDEX users_username_lower_key ON users (lower(username));
CREATE UNIQUE INDEX users_email_lower_key ON users (lower(email));
CREATE INDEX users_role_active_idx ON users (role) WHERE deleted_at IS NULL;
CREATE TRIGGER users_set_updated_at BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE sessions (
    id         TEXT        PRIMARY KEY, -- SHA-256 hex digest of the cookie token
    user_id    BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    csrf_token TEXT        NOT NULL CHECK (csrf_token <> ''),
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX sessions_user_id_idx ON sessions (user_id);
CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);

CREATE TABLE password_reset_tokens (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    token_hash TEXT        NOT NULL UNIQUE,
    user_id    BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX password_reset_tokens_user_id_idx ON password_reset_tokens (user_id);
CREATE INDEX password_reset_tokens_expires_at_idx ON password_reset_tokens (expires_at);

-- ---------------------------------------------------------------------------
-- Marketplace configuration
-- ---------------------------------------------------------------------------

CREATE TABLE store_settings (
    id             SMALLINT    PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    store_name     TEXT        NOT NULL CHECK (store_name <> ''),
    store_slug     TEXT        NOT NULL CHECK (store_slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    description    TEXT        NOT NULL DEFAULT '',
    contact_email  TEXT        NOT NULL CHECK (contact_email <> ''),
    initialized_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TRIGGER store_settings_set_updated_at BEFORE UPDATE ON store_settings
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Partners are the vendors whose products are listed on the marketplace.
CREATE TABLE partners (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name          TEXT        NOT NULL CHECK (name <> ''),
    slug          TEXT        NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9]+([_-][a-z0-9]+)*$'),
    contact_email TEXT        NOT NULL DEFAULT '',
    status        TEXT        NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TRIGGER partners_set_updated_at BEFORE UPDATE ON partners
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ---------------------------------------------------------------------------
-- Catalog
-- ---------------------------------------------------------------------------

CREATE TABLE categories (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name        TEXT        NOT NULL CHECK (name <> ''),
    slug        TEXT        NOT NULL CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    description TEXT        NOT NULL DEFAULT '',
    parent_id   BIGINT      REFERENCES categories (id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at  TIMESTAMPTZ,
    CHECK (parent_id IS DISTINCT FROM id)
);
CREATE UNIQUE INDEX categories_slug_active_key ON categories (slug) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX categories_name_active_key ON categories (lower(name)) WHERE deleted_at IS NULL;
CREATE INDEX categories_parent_id_idx ON categories (parent_id);
CREATE TRIGGER categories_set_updated_at BEFORE UPDATE ON categories
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE products (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    partner_id    BIGINT      REFERENCES partners (id) ON DELETE RESTRICT,
    category_id   BIGINT      REFERENCES categories (id) ON DELETE SET NULL,
    name          TEXT        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 200),
    slug          TEXT        NOT NULL CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    description   TEXT        NOT NULL DEFAULT '',
    price_cents   BIGINT      NOT NULL CHECK (price_cents >= 0),
    currency      CHAR(3)     NOT NULL DEFAULT 'USD',
    search_vector TSVECTOR GENERATED ALWAYS AS (
        setweight(to_tsvector('simple'::regconfig, coalesce(name, '')), 'A') ||
        setweight(to_tsvector('simple'::regconfig, coalesce(description, '')), 'B')
    ) STORED,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at    TIMESTAMPTZ
);
CREATE UNIQUE INDEX products_slug_active_key ON products (slug) WHERE deleted_at IS NULL;
CREATE INDEX products_partner_id_idx ON products (partner_id);
CREATE INDEX products_category_id_idx ON products (category_id);
CREATE INDEX products_name_active_idx ON products (name) WHERE deleted_at IS NULL;
CREATE INDEX products_search_vector_idx ON products USING GIN (search_vector);
CREATE TRIGGER products_set_updated_at BEFORE UPDATE ON products
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE product_images (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    product_id BIGINT      NOT NULL REFERENCES products (id) ON DELETE CASCADE,
    url        TEXT        NOT NULL CHECK (url <> ''),
    alt_text   TEXT        NOT NULL DEFAULT '',
    position   INTEGER     NOT NULL DEFAULT 0 CHECK (position >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX product_images_product_position_idx ON product_images (product_id, position, id);

-- ---------------------------------------------------------------------------
-- Inventory
--
-- inventory_stocks is the single source of truth for stock. Available stock
-- is on_hand - reserved - allocated:
--   reserved  - held by open checkouts (stock_reservations)
--   allocated - committed to placed orders that have not shipped yet
-- ---------------------------------------------------------------------------

CREATE TABLE warehouses (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name       TEXT        NOT NULL CHECK (name <> ''),
    slug       TEXT        NOT NULL UNIQUE,
    is_default BOOLEAN     NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX warehouses_single_default_key ON warehouses (is_default) WHERE is_default;
CREATE TRIGGER warehouses_set_updated_at BEFORE UPDATE ON warehouses
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

INSERT INTO warehouses (name, slug, is_default) VALUES ('Default Warehouse', 'default', true);

CREATE TABLE inventory_stocks (
    product_id         BIGINT      NOT NULL REFERENCES products (id) ON DELETE CASCADE,
    warehouse_id       BIGINT      NOT NULL REFERENCES warehouses (id) ON DELETE RESTRICT,
    on_hand_quantity   INTEGER     NOT NULL DEFAULT 0 CHECK (on_hand_quantity >= 0),
    reserved_quantity  INTEGER     NOT NULL DEFAULT 0 CHECK (reserved_quantity >= 0),
    allocated_quantity INTEGER     NOT NULL DEFAULT 0 CHECK (allocated_quantity >= 0),
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (product_id, warehouse_id),
    CONSTRAINT inventory_stocks_not_oversold
        CHECK (reserved_quantity + allocated_quantity <= on_hand_quantity)
);
CREATE INDEX inventory_stocks_warehouse_id_idx ON inventory_stocks (warehouse_id);
CREATE TRIGGER inventory_stocks_set_updated_at BEFORE UPDATE ON inventory_stocks
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE stock_reservations (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    product_id      BIGINT      NOT NULL,
    warehouse_id    BIGINT      NOT NULL,
    user_id         BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    reservation_key TEXT        NOT NULL,
    quantity        INTEGER     NOT NULL CHECK (quantity > 0),
    status          TEXT        NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'released', 'expired', 'converted')),
    expires_at      TIMESTAMPTZ NOT NULL,
    released_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (product_id, warehouse_id) REFERENCES inventory_stocks (product_id, warehouse_id) ON DELETE CASCADE
);
CREATE INDEX stock_reservations_active_expiry_idx ON stock_reservations (expires_at) WHERE status = 'active';
CREATE INDEX stock_reservations_active_key_idx ON stock_reservations (user_id, reservation_key) WHERE status = 'active';
CREATE INDEX stock_reservations_product_idx ON stock_reservations (product_id, warehouse_id);
CREATE TRIGGER stock_reservations_set_updated_at BEFORE UPDATE ON stock_reservations
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE stock_movements (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    product_id     BIGINT      NOT NULL,
    warehouse_id   BIGINT      NOT NULL,
    movement_type  TEXT        NOT NULL CHECK (movement_type IN (
        'initial', 'adjustment', 'reservation', 'release', 'allocation', 'deallocation', 'shipment', 'return'
    )),
    quantity_delta INTEGER     NOT NULL,
    reference      TEXT        NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (product_id, warehouse_id) REFERENCES inventory_stocks (product_id, warehouse_id) ON DELETE CASCADE
);
CREATE INDEX stock_movements_product_created_idx ON stock_movements (product_id, created_at);

-- ---------------------------------------------------------------------------
-- Cart and checkout
-- ---------------------------------------------------------------------------

CREATE TABLE carts (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    BIGINT      NOT NULL UNIQUE REFERENCES users (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TRIGGER carts_set_updated_at BEFORE UPDATE ON carts
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE cart_items (
    cart_id    BIGINT      NOT NULL REFERENCES carts (id) ON DELETE CASCADE,
    product_id BIGINT      NOT NULL REFERENCES products (id) ON DELETE CASCADE,
    quantity   INTEGER     NOT NULL CHECK (quantity BETWEEN 1 AND 999),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (cart_id, product_id)
);
CREATE INDEX cart_items_product_id_idx ON cart_items (product_id);
CREATE TRIGGER cart_items_set_updated_at BEFORE UPDATE ON cart_items
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ---------------------------------------------------------------------------
-- Orders
-- ---------------------------------------------------------------------------

CREATE TABLE orders (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id          BIGINT      NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    status           TEXT        NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'processing', 'shipped', 'delivered', 'cancelled')),
    partner_status   TEXT        NOT NULL DEFAULT 'new'
        CHECK (partner_status IN ('new', 'accepted', 'packing', 'dispatched', 'completed', 'cancelled')),
    delivery_status  TEXT        NOT NULL DEFAULT 'queued'
        CHECK (delivery_status IN ('queued', 'processing', 'shipped', 'delivered')),
    -- Tracks what the order holds in inventory: allocated stock is committed but
    -- still on hand, shipped stock has left on_hand, released stock went back.
    inventory_state  TEXT        NOT NULL DEFAULT 'allocated'
        CHECK (inventory_state IN ('allocated', 'shipped', 'released')),
    currency         CHAR(3)     NOT NULL DEFAULT 'USD',
    subtotal_cents   BIGINT      NOT NULL CHECK (subtotal_cents >= 0),
    shipping_cents   BIGINT      NOT NULL DEFAULT 0 CHECK (shipping_cents >= 0),
    tax_cents        BIGINT      NOT NULL DEFAULT 0 CHECK (tax_cents >= 0),
    total_cents      BIGINT      NOT NULL CHECK (total_cents >= 0),
    delivery_address TEXT        NOT NULL CHECK (delivery_address <> ''),
    delivery_notice  TEXT        NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT orders_total_matches CHECK (total_cents = subtotal_cents + shipping_cents + tax_cents)
);
CREATE INDEX orders_user_created_idx ON orders (user_id, created_at DESC);
CREATE INDEX orders_status_idx ON orders (status);
CREATE INDEX orders_partner_status_created_idx ON orders (partner_status, created_at);
CREATE INDEX orders_created_at_idx ON orders (created_at DESC);
CREATE TRIGGER orders_set_updated_at BEFORE UPDATE ON orders
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE order_items (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    order_id         BIGINT      NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
    product_id       BIGINT      NOT NULL REFERENCES products (id) ON DELETE RESTRICT,
    partner_id       BIGINT      REFERENCES partners (id) ON DELETE SET NULL,
    warehouse_id     BIGINT      NOT NULL REFERENCES warehouses (id) ON DELETE RESTRICT,
    product_name     TEXT        NOT NULL,
    quantity         INTEGER     NOT NULL CHECK (quantity > 0),
    unit_price_cents BIGINT      NOT NULL CHECK (unit_price_cents >= 0),
    line_total_cents BIGINT GENERATED ALWAYS AS (unit_price_cents * quantity) STORED,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX order_items_order_id_idx ON order_items (order_id);
CREATE INDEX order_items_product_id_idx ON order_items (product_id);
CREATE INDEX order_items_partner_id_idx ON order_items (partner_id);

CREATE TABLE checkouts (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id          BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token            TEXT        NOT NULL UNIQUE,
    status           TEXT        NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'completed', 'expired', 'cancelled')),
    currency         CHAR(3)     NOT NULL DEFAULT 'USD',
    payment_method   TEXT        NOT NULL DEFAULT 'cash_on_delivery',
    subtotal_cents   BIGINT      NOT NULL DEFAULT 0 CHECK (subtotal_cents >= 0),
    shipping_cents   BIGINT      NOT NULL DEFAULT 0 CHECK (shipping_cents >= 0),
    tax_cents        BIGINT      NOT NULL DEFAULT 0 CHECK (tax_cents >= 0),
    total_cents      BIGINT      NOT NULL DEFAULT 0 CHECK (total_cents >= 0),
    delivery_address TEXT        NOT NULL DEFAULT '',
    expires_at       TIMESTAMPTZ NOT NULL,
    completed_at     TIMESTAMPTZ,
    order_id         BIGINT      UNIQUE REFERENCES orders (id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT checkouts_total_matches CHECK (total_cents = subtotal_cents + shipping_cents + tax_cents)
);
CREATE INDEX checkouts_user_status_idx ON checkouts (user_id, status, updated_at DESC);
CREATE INDEX checkouts_open_expiry_idx ON checkouts (expires_at) WHERE status = 'open';
CREATE TRIGGER checkouts_set_updated_at BEFORE UPDATE ON checkouts
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE checkout_lines (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    checkout_id      BIGINT      NOT NULL REFERENCES checkouts (id) ON DELETE CASCADE,
    product_id       BIGINT      NOT NULL REFERENCES products (id) ON DELETE CASCADE,
    product_name     TEXT        NOT NULL,
    quantity         INTEGER     NOT NULL CHECK (quantity > 0),
    unit_price_cents BIGINT      NOT NULL CHECK (unit_price_cents >= 0),
    line_total_cents BIGINT GENERATED ALWAYS AS (unit_price_cents * quantity) STORED,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (checkout_id, product_id)
);

-- ---------------------------------------------------------------------------
-- Payments
-- ---------------------------------------------------------------------------

CREATE TABLE payments (
    id                 BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    order_id           BIGINT      NOT NULL REFERENCES orders (id) ON DELETE RESTRICT,
    user_id            BIGINT      NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    method             TEXT        NOT NULL CHECK (method <> ''),
    provider           TEXT        NOT NULL CHECK (provider <> ''),
    status             TEXT        NOT NULL CHECK (status IN ('pending', 'captured', 'failed', 'refunded')),
    currency           CHAR(3)     NOT NULL DEFAULT 'USD',
    amount_cents       BIGINT      NOT NULL CHECK (amount_cents >= 0),
    external_reference TEXT        UNIQUE,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX payments_order_id_idx ON payments (order_id);
CREATE INDEX payments_user_id_idx ON payments (user_id);
CREATE TRIGGER payments_set_updated_at BEFORE UPDATE ON payments
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE payment_attempts (
    id                 BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    payment_id         BIGINT      NOT NULL REFERENCES payments (id) ON DELETE CASCADE,
    status             TEXT        NOT NULL CHECK (status IN ('pending', 'completed', 'failed')),
    request_reference  TEXT        NOT NULL DEFAULT '',
    external_reference TEXT,
    error_message      TEXT,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX payment_attempts_payment_id_idx ON payment_attempts (payment_id);

CREATE TABLE idempotency_keys (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id         BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    operation       TEXT        NOT NULL,
    idempotency_key TEXT        NOT NULL,
    order_id        BIGINT      REFERENCES orders (id) ON DELETE SET NULL,
    payment_id      BIGINT      REFERENCES payments (id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at    TIMESTAMPTZ,
    UNIQUE (user_id, operation, idempotency_key)
);

-- ---------------------------------------------------------------------------
-- Audit
-- ---------------------------------------------------------------------------

CREATE TABLE audit_logs (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor_user_id BIGINT      REFERENCES users (id) ON DELETE SET NULL,
    action        TEXT        NOT NULL CHECK (action <> ''),
    target_type   TEXT        NOT NULL CHECK (target_type <> ''),
    target_id     BIGINT,
    details       TEXT        NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX audit_logs_created_at_idx ON audit_logs (created_at DESC, id DESC);
CREATE INDEX audit_logs_target_idx ON audit_logs (target_type, target_id);

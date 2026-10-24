-- Courier accounts are separate from platform staff and marketplace partners.
ALTER TABLE users DROP CONSTRAINT users_role_check;
ALTER TABLE users ADD CONSTRAINT users_role_check
    CHECK (role IN ('user', 'staff', 'admin', 'partner', 'courier'));

-- One current courier assignment per order. Reassignment history is retained
-- in courier_delivery_events, and user rows are soft-deleted rather than
-- removed so assignment and event references remain valid.
CREATE TABLE courier_deliveries (
    id                 BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    order_id           BIGINT      NOT NULL UNIQUE REFERENCES orders (id) ON DELETE RESTRICT,
    courier_user_id    BIGINT      NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    assigned_by_user_id BIGINT     NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    status             TEXT        NOT NULL DEFAULT 'assigned'
        CHECK (status IN ('assigned', 'picked_up', 'out_for_delivery', 'delivered', 'delivery_failed')),
    failure_note       TEXT        NOT NULL DEFAULT ''
        CHECK (failure_note IN ('', 'Recipient unavailable', 'Address issue', 'Item damaged', 'Recipient declined', 'Other')),
    assigned_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    picked_up_at       TIMESTAMPTZ,
    out_for_delivery_at TIMESTAMPTZ,
    delivered_at       TIMESTAMPTZ,
    failed_at          TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (status NOT IN ('picked_up', 'out_for_delivery', 'delivered') OR picked_up_at IS NOT NULL),
    CHECK (status <> 'out_for_delivery' OR out_for_delivery_at IS NOT NULL),
    CHECK (status <> 'delivered' OR delivered_at IS NOT NULL),
    CHECK (status <> 'delivery_failed' OR failed_at IS NOT NULL)
);
CREATE INDEX courier_deliveries_courier_status_idx
    ON courier_deliveries (courier_user_id, status, assigned_at DESC);
CREATE INDEX courier_deliveries_status_assigned_idx
    ON courier_deliveries (status, assigned_at, id);
CREATE TRIGGER courier_deliveries_set_updated_at BEFORE UPDATE ON courier_deliveries
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Append-only timeline: who assigned/reassigned the delivery and which courier
-- recorded each handoff, failure, or completion.
CREATE TABLE courier_delivery_events (
    id                         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    delivery_id                BIGINT      NOT NULL REFERENCES courier_deliveries (id) ON DELETE RESTRICT,
    actor_user_id              BIGINT      NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    action                     TEXT        NOT NULL
        CHECK (action IN ('assigned', 'reassigned', 'picked_up', 'out_for_delivery', 'delivered', 'delivery_failed')),
    previous_status            TEXT        NOT NULL DEFAULT '',
    status                     TEXT        NOT NULL,
    previous_courier_user_id   BIGINT      REFERENCES users (id) ON DELETE RESTRICT,
    courier_user_id            BIGINT      NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    note                       TEXT        NOT NULL DEFAULT ''
        CHECK (note IN ('', 'Recipient unavailable', 'Address issue', 'Item damaged', 'Recipient declined', 'Other')),
    created_at                 TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX courier_delivery_events_delivery_created_idx
    ON courier_delivery_events (delivery_id, created_at, id);

CREATE FUNCTION protect_courier_delivery_events() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'courier delivery events are append-only';
END;
$$;
CREATE TRIGGER courier_delivery_events_append_only
    BEFORE UPDATE OR DELETE ON courier_delivery_events
    FOR EACH ROW EXECUTE FUNCTION protect_courier_delivery_events();

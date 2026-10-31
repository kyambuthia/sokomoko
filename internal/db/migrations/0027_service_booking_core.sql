CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE service_resources (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    service_listing_id BIGINT NOT NULL REFERENCES service_listings(id) ON DELETE CASCADE,
    name TEXT NOT NULL CHECK (char_length(name) BETWEEN 1 AND 80),
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (service_listing_id, name),
    UNIQUE (id, service_listing_id)
);
CREATE INDEX service_resources_listing_idx ON service_resources(service_listing_id, id) WHERE active;
CREATE TRIGGER service_resources_set_updated_at BEFORE UPDATE ON service_resources
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

INSERT INTO service_resources(service_listing_id, name)
SELECT id, 'Primary resource' FROM service_listings;

CREATE FUNCTION create_default_service_resource() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO service_resources(service_listing_id, name)
    VALUES (NEW.id, 'Primary resource')
    ON CONFLICT (service_listing_id, name) DO NOTHING;
    RETURN NEW;
END;
$$;
CREATE TRIGGER service_listings_default_resource AFTER INSERT ON service_listings
    FOR EACH ROW EXECUTE FUNCTION create_default_service_resource();

CREATE TABLE service_bookings (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    service_listing_id BIGINT NOT NULL REFERENCES service_listings(id) ON DELETE RESTRICT,
    resource_id BIGINT NOT NULL,
    customer_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    service_title TEXT NOT NULL CHECK (char_length(service_title) BETWEEN 3 AND 120),
    service_area TEXT NOT NULL CHECK (char_length(service_area) BETWEEN 2 AND 120),
    delivery_mode TEXT NOT NULL CHECK (delivery_mode IN ('premises','mobile','remote')),
    duration_minutes INT NOT NULL CHECK (duration_minutes BETWEEN 5 AND 480),
    price_mode TEXT NOT NULL CHECK (price_mode IN ('fixed','from','quote')),
    price_cents BIGINT NOT NULL CHECK (price_cents BETWEEN 0 AND 100000000),
    currency TEXT NOT NULL CHECK (currency IN ('KES','USD')),
    timezone TEXT NOT NULL CHECK (char_length(timezone) BETWEEN 1 AND 64),
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL,
    time_range TSTZRANGE GENERATED ALWAYS AS (tstzrange(starts_at, ends_at, '[)')) STORED,
    status TEXT NOT NULL DEFAULT 'requested'
        CHECK (status IN ('requested','confirmed','rejected','cancelled','completed','expired','in_progress','no_show')),
    response_deadline TIMESTAMPTZ NOT NULL,
    idempotency_key TEXT NOT NULL CHECK (char_length(idempotency_key) BETWEEN 16 AND 128),
    request_hash BYTEA NOT NULL CHECK (octet_length(request_hash) = 32),
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (ends_at > starts_at),
    CHECK (response_deadline <= starts_at),
    CHECK (price_mode <> 'quote' OR price_cents = 0),
    UNIQUE (customer_id, idempotency_key),
    FOREIGN KEY (resource_id, service_listing_id)
        REFERENCES service_resources(id, service_listing_id) ON DELETE RESTRICT,
    EXCLUDE USING gist (resource_id WITH =, time_range WITH &&)
        WHERE (status IN ('requested','confirmed','in_progress'))
);
CREATE INDEX service_bookings_customer_idx ON service_bookings(customer_id, created_at DESC, id DESC);
CREATE INDEX service_bookings_partner_queue_idx ON service_bookings(service_listing_id, starts_at, id)
    WHERE status = 'requested';
CREATE INDEX service_bookings_response_deadline_idx ON service_bookings(response_deadline)
    WHERE status = 'requested';
CREATE TRIGGER service_bookings_set_updated_at BEFORE UPDATE ON service_bookings
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE service_booking_events (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    booking_id BIGINT NOT NULL REFERENCES service_bookings(id) ON DELETE RESTRICT,
    actor_user_id BIGINT REFERENCES users(id) ON DELETE RESTRICT,
    from_status TEXT CHECK (from_status IN ('requested','confirmed','rejected','cancelled','completed','expired','in_progress','no_show')),
    to_status TEXT NOT NULL CHECK (to_status IN ('requested','confirmed','rejected','cancelled','completed','expired','in_progress','no_show')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX service_booking_events_booking_idx ON service_booking_events(booking_id, id);

CREATE FUNCTION guard_service_booking_update() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF ROW(NEW.service_listing_id, NEW.resource_id, NEW.customer_id, NEW.service_title,
           NEW.service_area, NEW.delivery_mode, NEW.duration_minutes, NEW.price_mode,
           NEW.price_cents, NEW.currency, NEW.timezone,
           NEW.starts_at, NEW.ends_at, NEW.response_deadline, NEW.idempotency_key, NEW.request_hash)
       IS DISTINCT FROM
       ROW(OLD.service_listing_id, OLD.resource_id, OLD.customer_id, OLD.service_title,
           OLD.service_area, OLD.delivery_mode, OLD.duration_minutes, OLD.price_mode,
           OLD.price_cents, OLD.currency, OLD.timezone,
           OLD.starts_at, OLD.ends_at, OLD.response_deadline, OLD.idempotency_key, OLD.request_hash) THEN
        RAISE EXCEPTION 'service booking snapshot is immutable'
            USING ERRCODE = '23514', CONSTRAINT = 'service_bookings_immutable_snapshot';
    END IF;
    IF NEW.status IS DISTINCT FROM OLD.status AND NOT (
        (OLD.status = 'requested' AND NEW.status IN ('confirmed','rejected','cancelled','expired')) OR
        (OLD.status = 'confirmed' AND NEW.status IN ('in_progress','cancelled','completed','no_show')) OR
        (OLD.status = 'in_progress' AND NEW.status IN ('completed','no_show'))
    ) THEN
        RAISE EXCEPTION 'invalid service booking status transition: % to %', OLD.status, NEW.status
            USING ERRCODE = '23514', CONSTRAINT = 'service_bookings_status_transition';
    END IF;
    IF NEW.status IS DISTINCT FROM OLD.status AND NEW.version <> OLD.version + 1 THEN
        RAISE EXCEPTION 'service booking version must increment with status'
            USING ERRCODE = '23514', CONSTRAINT = 'service_bookings_version_transition';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER service_bookings_guard_update BEFORE UPDATE ON service_bookings
    FOR EACH ROW EXECUTE FUNCTION guard_service_booking_update();

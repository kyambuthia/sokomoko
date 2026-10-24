-- Service discovery is separate from product stock and appointment booking.
-- PublicArea is intentionally a broad, seller-published area, never a private
-- visit address or precise customer/provider coordinate.
CREATE TABLE service_listings (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    partner_id BIGINT NOT NULL REFERENCES partners(id) ON DELETE RESTRICT,
    slug TEXT NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$' AND char_length(slug) <= 160),
    title TEXT NOT NULL CHECK (char_length(title) BETWEEN 3 AND 120),
    description TEXT NOT NULL CHECK (char_length(description) BETWEEN 10 AND 3000),
    category TEXT NOT NULL CHECK (category IN ('hair','beauty','dental','wellness','home','other')),
    duration_minutes INT NOT NULL CHECK (duration_minutes BETWEEN 5 AND 480),
    price_mode TEXT NOT NULL CHECK (price_mode IN ('fixed','from','quote')),
    price_cents BIGINT NOT NULL CHECK (price_cents BETWEEN 0 AND 100000000),
    currency TEXT NOT NULL CHECK (currency IN ('KES','USD')),
    public_area TEXT NOT NULL CHECK (char_length(public_area) BETWEEN 2 AND 120),
    delivery_mode TEXT NOT NULL CHECK (delivery_mode IN ('premises','mobile','remote')),
    state TEXT NOT NULL DEFAULT 'draft' CHECK (state IN ('draft','published','archived')),
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (price_mode <> 'quote' OR price_cents = 0)
);
CREATE INDEX service_listings_partner_idx ON service_listings(partner_id,updated_at DESC,id DESC);
CREATE INDEX service_listings_discovery_idx ON service_listings(category,lower(title),id) WHERE state='published';
CREATE TRIGGER service_listings_set_updated_at BEFORE UPDATE ON service_listings
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

ALTER TABLE service_listings
    ADD COLUMN IF NOT EXISTS public_lat DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS public_lng DOUBLE PRECISION;

ALTER TABLE service_listings
    DROP CONSTRAINT IF EXISTS service_listings_public_coords_check;

ALTER TABLE service_listings
    ADD CONSTRAINT service_listings_public_coords_check CHECK (
        (public_lat IS NULL AND public_lng IS NULL)
        OR (
            delivery_mode = 'premises'
            AND public_lat BETWEEN -90 AND 90
            AND public_lng BETWEEN -180 AND 180
        )
    );

CREATE INDEX IF NOT EXISTS idx_service_listings_public_coords
    ON service_listings (public_lat, public_lng)
    WHERE state = 'published' AND public_lat IS NOT NULL AND public_lng IS NOT NULL;

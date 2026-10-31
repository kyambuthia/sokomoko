ALTER TABLE service_listings
    ADD COLUMN timezone TEXT NOT NULL DEFAULT 'Africa/Nairobi'
        CHECK (char_length(timezone) BETWEEN 1 AND 64);

CREATE TABLE service_weekly_hours (
    service_listing_id BIGINT NOT NULL REFERENCES service_listings(id) ON DELETE CASCADE,
    weekday SMALLINT NOT NULL CHECK (weekday BETWEEN 0 AND 6),
    opens_at TIME NOT NULL,
    closes_at TIME NOT NULL,
    PRIMARY KEY (service_listing_id, weekday),
    CHECK (opens_at < closes_at)
);

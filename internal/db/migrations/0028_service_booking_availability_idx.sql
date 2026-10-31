CREATE INDEX service_bookings_listing_active_range_idx
    ON service_bookings(service_listing_id, starts_at, ends_at)
    WHERE status IN ('requested','confirmed','in_progress');

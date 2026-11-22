CREATE TABLE IF NOT EXISTS traffic_rollups (
    bucket_hour TIMESTAMPTZ NOT NULL,
    client_kind TEXT NOT NULL,
    surface TEXT NOT NULL,
    request_count BIGINT NOT NULL DEFAULT 0,
    error_4xx_count BIGINT NOT NULL DEFAULT 0,
    error_5xx_count BIGINT NOT NULL DEFAULT 0,
    rate_limited_count BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (bucket_hour, client_kind, surface)
);

CREATE INDEX IF NOT EXISTS idx_traffic_rollups_bucket_hour
    ON traffic_rollups (bucket_hour DESC);

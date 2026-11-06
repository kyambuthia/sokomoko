CREATE TABLE product_video_blobs (
    product_id   BIGINT PRIMARY KEY REFERENCES products (id) ON DELETE CASCADE,
    content_type TEXT        NOT NULL CHECK (content_type IN ('video/mp4', 'video/webm')),
    data         BYTEA       NOT NULL CHECK (octet_length(data) BETWEEN 1 AND 10485760),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE product_image_blobs (
    product_image_id BIGINT PRIMARY KEY REFERENCES product_images (id) ON DELETE CASCADE,
    content_type     TEXT        NOT NULL CHECK (content_type IN ('image/jpeg', 'image/png')),
    data             BYTEA       NOT NULL CHECK (octet_length(data) BETWEEN 1 AND 262144)
);

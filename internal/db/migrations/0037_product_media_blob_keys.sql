ALTER TABLE product_image_blobs
    ADD COLUMN IF NOT EXISTS blob_key TEXT,
    ALTER COLUMN data DROP NOT NULL;

ALTER TABLE product_video_blobs
    ADD COLUMN IF NOT EXISTS blob_key TEXT,
    ALTER COLUMN data DROP NOT NULL;

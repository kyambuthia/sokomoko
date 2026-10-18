-- Substring fallback in catalog search needs indexes as the catalog grows.
-- pg_trgm is a trusted PostgreSQL extension and its GIN opclass supports ILIKE.
CREATE EXTENSION IF NOT EXISTS pg_trgm WITH SCHEMA public;

CREATE INDEX products_name_trgm_idx ON products USING GIN (name public.gin_trgm_ops)
    WHERE deleted_at IS NULL;
CREATE INDEX products_description_trgm_idx ON products USING GIN (description public.gin_trgm_ops)
    WHERE deleted_at IS NULL;

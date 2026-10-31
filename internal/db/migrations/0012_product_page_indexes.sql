-- Keep paginated catalog and workspace listings ordered without sorting every
-- active product for each page request.
CREATE INDEX products_active_name_id_idx ON products (lower(name), id)
    WHERE deleted_at IS NULL;
CREATE INDEX products_active_partner_name_id_idx ON products (partner_id, lower(name), id)
    WHERE deleted_at IS NULL;

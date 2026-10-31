-- Speed substring searches on the active admin team list.
CREATE INDEX users_active_username_trgm_idx
    ON users USING GIN (username public.gin_trgm_ops)
    WHERE deleted_at IS NULL;

CREATE INDEX users_active_email_trgm_idx
    ON users USING GIN (email public.gin_trgm_ops)
    WHERE deleted_at IS NULL;

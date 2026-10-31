-- Serve the admin restore list in its newest-deactivated-first order.
CREATE INDEX users_deactivated_page_idx ON users (deleted_at DESC, id DESC)
    WHERE deleted_at IS NOT NULL;

-- Keep the active team member page in role and username order.
CREATE INDEX users_active_team_page_idx
    ON users (role, lower(username), id)
    WHERE deleted_at IS NULL;

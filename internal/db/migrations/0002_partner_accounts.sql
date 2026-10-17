-- Preserve partner logins and ownership from the versioned API branch.
-- Partner records and login accounts have independent identifiers.
ALTER TABLE users DROP CONSTRAINT users_role_check;
ALTER TABLE users ADD CONSTRAINT users_role_check
    CHECK (role IN ('user', 'staff', 'admin', 'partner'));
CREATE TABLE partner_users (
    user_id BIGINT PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    partner_id BIGINT NOT NULL REFERENCES partners (id) ON DELETE CASCADE
);
CREATE INDEX partner_users_partner_id_idx ON partner_users (partner_id);

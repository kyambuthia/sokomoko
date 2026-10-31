-- Keep unresolved reports in chronological order for the moderator queue.
CREATE INDEX chat_reports_open_page_idx
    ON chat_reports (created_at, id)
    WHERE resolved_at IS NULL;

CREATE INDEX chat_encrypted_reports_open_page_idx
    ON chat_encrypted_reports (created_at, id)
    WHERE resolved_at IS NULL;

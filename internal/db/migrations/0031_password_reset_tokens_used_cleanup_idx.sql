CREATE INDEX password_reset_tokens_used_cleanup_idx ON password_reset_tokens (id) WHERE used_at IS NOT NULL;

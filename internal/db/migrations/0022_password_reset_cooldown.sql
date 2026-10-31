-- Persist reset-request throttling across instances and identifier aliases.
ALTER TABLE users ADD COLUMN password_reset_requested_at TIMESTAMPTZ;

-- The application never receives MLS private keys, group state, or plaintext.
CREATE TABLE chat_devices (
    id TEXT PRIMARY KEY CHECK (id ~ '^[a-f0-9]{32}$'),
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    name TEXT NOT NULL CHECK (char_length(name) BETWEEN 1 AND 60),
    public_key BYTEA NOT NULL CHECK (octet_length(public_key)=32),
    approved_by TEXT REFERENCES chat_devices(id) ON DELETE RESTRICT,
    approved_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(user_id,public_key)
);
CREATE INDEX chat_devices_user_idx ON chat_devices(user_id);
CREATE TABLE chat_device_packages (
    id TEXT PRIMARY KEY CHECK (id ~ '^[a-f0-9]{32}$'),
    device_id TEXT NOT NULL REFERENCES chat_devices(id) ON DELETE RESTRICT,
    package BYTEA NOT NULL CHECK (octet_length(package) BETWEEN 64 AND 16384),
    claimed_at TIMESTAMPTZ,
    claimed_by TEXT REFERENCES chat_devices(id),
    conversation_id BIGINT REFERENCES chat_conversations(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX chat_packages_available_idx ON chat_device_packages(device_id) WHERE claimed_at IS NULL;
CREATE TABLE chat_encrypted_sessions (
    id TEXT PRIMARY KEY CHECK (id ~ '^[a-f0-9]{32}$'),
    conversation_id BIGINT NOT NULL REFERENCES chat_conversations(id) ON DELETE RESTRICT,
    creator_device_id TEXT NOT NULL REFERENCES chat_devices(id) ON DELETE RESTRICT,
    roster JSONB NOT NULL CHECK (jsonb_typeof(roster)='array'),
    roster_hash TEXT NOT NULL CHECK (roster_hash ~ '^[a-f0-9]{64}$'),
    welcome BYTEA NOT NULL CHECK (octet_length(welcome) BETWEEN 64 AND 262144),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE chat_device_packages ADD COLUMN used_session_id TEXT REFERENCES chat_encrypted_sessions(id) ON DELETE RESTRICT;
ALTER TABLE chat_conversations ADD COLUMN encrypted_session_id TEXT REFERENCES chat_encrypted_sessions(id) ON DELETE RESTRICT;
CREATE TABLE chat_encrypted_events (
    id BIGINT DEFAULT nextval(pg_get_serial_sequence('chat_messages','id')) PRIMARY KEY,
    conversation_id BIGINT NOT NULL REFERENCES chat_conversations(id) ON DELETE RESTRICT,
    session_id TEXT NOT NULL REFERENCES chat_encrypted_sessions(id) ON DELETE RESTRICT,
    sender_device_id TEXT NOT NULL REFERENCES chat_devices(id) ON DELETE RESTRICT,
    kind TEXT NOT NULL CHECK (kind IN ('message','update')),
    ciphertext BYTEA NOT NULL CHECK (octet_length(ciphertext) BETWEEN 32 AND 65536),
    nonce TEXT NOT NULL CHECK (nonce ~ '^[a-f0-9]{32}$'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(sender_device_id,nonce)
);
CREATE INDEX chat_encrypted_events_page_idx ON chat_encrypted_events(conversation_id,id);
CREATE TABLE chat_encrypted_requests (
    device_id TEXT NOT NULL REFERENCES chat_devices(id) ON DELETE RESTRICT,
    nonce TEXT NOT NULL CHECK (nonce ~ '^[a-f0-9]{32}$'),
    digest TEXT NOT NULL,
    response JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY(device_id,nonce)
);
CREATE TABLE chat_encrypted_reports (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    event_id BIGINT NOT NULL REFERENCES chat_encrypted_events(id) ON DELETE RESTRICT,
    reporter_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    reason TEXT NOT NULL CHECK (reason IN ('spam','harassment','scam','personal_information','other')),
    disclosed_text TEXT NOT NULL CHECK (char_length(disclosed_text) BETWEEN 1 AND 4000 AND octet_length(disclosed_text)<=16000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at TIMESTAMPTZ,
    resolved_by_user_id BIGINT REFERENCES users(id) ON DELETE RESTRICT,
    UNIQUE(event_id,reporter_user_id)
);

CREATE TABLE chat_encrypted_reads (
 conversation_id BIGINT NOT NULL REFERENCES chat_conversations(id) ON DELETE CASCADE,
 user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 last_event_id BIGINT NOT NULL REFERENCES chat_encrypted_events(id) ON DELETE CASCADE,
 PRIMARY KEY(conversation_id,user_id)
);

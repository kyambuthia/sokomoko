-- Handles reuse users.username and its case-insensitive unique index.
-- A seller participant is a shop; access is derived from partner_users.
CREATE TABLE chat_conversations (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind TEXT NOT NULL CHECK (kind IN ('direct', 'seller', 'delivery')),
    customer_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    other_user_id BIGINT REFERENCES users(id) ON DELETE RESTRICT,
    partner_id BIGINT REFERENCES partners(id) ON DELETE RESTRICT,
    product_id BIGINT REFERENCES products(id) ON DELETE RESTRICT,
    order_id BIGINT REFERENCES orders(id) ON DELETE RESTRICT,
    delivery_id BIGINT REFERENCES courier_deliveries(id) ON DELETE RESTRICT,
    assignment_event_id BIGINT REFERENCES courier_delivery_events(id) ON DELETE RESTRICT,
    requester_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    state TEXT NOT NULL CHECK (state IN ('pending', 'accepted', 'declined')),
    last_message_id BIGINT NOT NULL DEFAULT 0 CHECK (last_message_id >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (
      (kind = 'direct' AND other_user_id IS NOT NULL AND customer_user_id < other_user_id
        AND partner_id IS NULL AND product_id IS NULL AND order_id IS NULL
        AND delivery_id IS NULL AND assignment_event_id IS NULL)
      OR (kind = 'seller' AND other_user_id IS NULL AND partner_id IS NOT NULL
        AND NOT (product_id IS NOT NULL AND order_id IS NOT NULL)
        AND delivery_id IS NULL AND assignment_event_id IS NULL)
      OR (kind = 'delivery' AND other_user_id IS NOT NULL AND customer_user_id <> other_user_id
        AND partner_id IS NULL AND product_id IS NULL AND order_id IS NOT NULL
        AND delivery_id IS NOT NULL AND assignment_event_id IS NOT NULL AND state = 'accepted')
    )
);
CREATE UNIQUE INDEX chat_direct_pair_key ON chat_conversations(customer_user_id, other_user_id) WHERE kind = 'direct';
CREATE UNIQUE INDEX chat_seller_context_key ON chat_conversations(customer_user_id, partner_id, COALESCE(product_id, 0), COALESCE(order_id, 0)) WHERE kind = 'seller';
CREATE UNIQUE INDEX chat_delivery_assignment_key ON chat_conversations(assignment_event_id) WHERE kind = 'delivery';
CREATE INDEX chat_customer_idx ON chat_conversations(customer_user_id, last_message_id DESC);
CREATE INDEX chat_other_idx ON chat_conversations(other_user_id, last_message_id DESC) WHERE other_user_id IS NOT NULL;
CREATE INDEX chat_partner_idx ON chat_conversations(partner_id, last_message_id DESC) WHERE partner_id IS NOT NULL;

CREATE TABLE chat_messages (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    conversation_id BIGINT NOT NULL REFERENCES chat_conversations(id) ON DELETE RESTRICT,
    sender_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    body TEXT NOT NULL CHECK (char_length(btrim(body)) BETWEEN 1 AND 4000 AND octet_length(body) <= 16000),
    client_nonce TEXT NOT NULL CHECK (client_nonce ~ '^[a-f0-9]{32}$'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (conversation_id, sender_user_id, client_nonce)
);
CREATE INDEX chat_messages_page_idx ON chat_messages(conversation_id, id DESC);
CREATE INDEX chat_messages_sender_time_idx ON chat_messages(sender_user_id, created_at DESC);

CREATE TABLE chat_reads (
    conversation_id BIGINT NOT NULL REFERENCES chat_conversations(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    last_read_message_id BIGINT NOT NULL REFERENCES chat_messages(id) ON DELETE CASCADE,
    PRIMARY KEY(conversation_id, user_id)
);

-- Each end of a block is either an individual or a shop. Blocks apply across
-- contexts, so another product/order cannot bypass a blocked relationship.
CREATE TABLE chat_blocks (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    owner_user_id BIGINT REFERENCES users(id) ON DELETE CASCADE,
    owner_partner_id BIGINT REFERENCES partners(id) ON DELETE CASCADE,
    target_user_id BIGINT REFERENCES users(id) ON DELETE CASCADE,
    target_partner_id BIGINT REFERENCES partners(id) ON DELETE CASCADE,
    created_by_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((owner_user_id IS NULL) <> (owner_partner_id IS NULL)),
    CHECK ((target_user_id IS NULL) <> (target_partner_id IS NULL)),
    CHECK (owner_partner_id IS NULL OR target_partner_id IS NULL),
    CHECK (owner_user_id IS DISTINCT FROM target_user_id)
);
CREATE UNIQUE INDEX chat_block_pair_key ON chat_blocks(COALESCE(owner_user_id, 0), COALESCE(owner_partner_id, 0), COALESCE(target_user_id, 0), COALESCE(target_partner_id, 0));

CREATE TABLE chat_reports (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    conversation_id BIGINT NOT NULL REFERENCES chat_conversations(id) ON DELETE RESTRICT,
    message_id BIGINT NOT NULL REFERENCES chat_messages(id) ON DELETE RESTRICT,
    reporter_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    reason TEXT NOT NULL CHECK (reason IN ('spam', 'harassment', 'scam', 'personal_information', 'other')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at TIMESTAMPTZ,
    resolved_by_user_id BIGINT REFERENCES users(id) ON DELETE RESTRICT,
    UNIQUE(message_id, reporter_user_id)
);
CREATE INDEX chat_reports_open_idx ON chat_reports(id) WHERE resolved_at IS NULL;

-- Counters survive restarts and apply to every application process.
CREATE TABLE chat_rate_limits (
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    action TEXT NOT NULL,
    bucket BIGINT NOT NULL,
    hits INTEGER NOT NULL CHECK (hits > 0),
    expires_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY(user_id, action, bucket)
);
CREATE INDEX chat_rate_limits_expiry_idx ON chat_rate_limits(expires_at);

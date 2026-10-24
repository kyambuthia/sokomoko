-- Service enquiries retain the existing seller/shop participant and MLS model.
ALTER TABLE chat_conversations ADD COLUMN service_id BIGINT REFERENCES service_listings(id) ON DELETE RESTRICT;
ALTER TABLE chat_conversations ADD CONSTRAINT chat_service_context_check CHECK
 (service_id IS NULL OR (kind='seller' AND product_id IS NULL AND order_id IS NULL));
DROP INDEX chat_seller_context_key;
CREATE UNIQUE INDEX chat_seller_context_key ON chat_conversations
 (customer_user_id,partner_id,COALESCE(product_id,0),COALESCE(order_id,0),COALESCE(service_id,0)) WHERE kind='seller';
CREATE INDEX chat_service_idx ON chat_conversations(service_id) WHERE service_id IS NOT NULL;

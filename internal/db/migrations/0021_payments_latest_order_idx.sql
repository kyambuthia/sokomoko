-- Support the latest-payment lookup used by order reads.
DROP INDEX payments_order_id_idx;
CREATE INDEX payments_order_latest_idx ON payments (order_id, id DESC);

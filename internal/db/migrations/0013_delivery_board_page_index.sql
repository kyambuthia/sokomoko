-- Match the admin delivery board's pending predicate and oldest-first page order.
CREATE INDEX orders_pending_delivery_page_idx ON orders (created_at, id)
    WHERE status <> 'cancelled' AND delivery_status <> 'delivered';

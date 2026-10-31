-- Keep the operator exception queue in open-first, newest-first order.
CREATE INDEX payment_exceptions_page_idx
    ON payment_exceptions ((status = 'refunded'), created_at DESC, id DESC);

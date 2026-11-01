ALTER TABLE payment_exceptions
    DROP CONSTRAINT payment_exceptions_reason_check;

ALTER TABLE payment_exceptions
    ADD CONSTRAINT payment_exceptions_reason_check
    CHECK (reason IN ('late_capture','duplicate_charge','amount_mismatch','amount_unverified'));

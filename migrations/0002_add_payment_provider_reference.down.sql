ALTER TABLE payments
    DROP CONSTRAINT uq_payments_provider_reference,
    DROP COLUMN provider_reference;

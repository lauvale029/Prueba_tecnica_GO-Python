-- provider_reference es el identificador que asigna el proveedor externo
-- (ej. Nequi) a una operación de cobro — distinto de idempotency_key
-- (la genera el cliente de esta API) y de id (lo genera este sistema).
-- Es NULL hasta que el pago efectivamente se envía al proveedor.
ALTER TABLE payments
    ADD COLUMN provider_reference TEXT,
    ADD CONSTRAINT uq_payments_provider_reference UNIQUE (provider_reference);

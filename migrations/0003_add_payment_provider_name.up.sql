-- provider_name identifica CUÁL proveedor externo procesó un pago (ej.
-- "nequi", "bre-b", "simulated") — distinto de provider_reference, que
-- identifica LA OPERACIÓN específica dentro de ese proveedor. Es NULL
-- hasta que el pago se envía a un proveedor, igual que provider_reference.
ALTER TABLE payments
    ADD COLUMN provider_name TEXT;

-- gen_random_uuid() viene de pgcrypto; la aplicación siempre asigna el ID
-- explícitamente, este default solo sirve como comodidad para SQL manual
-- o pruebas ad-hoc.
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE merchants (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            TEXT NOT NULL,
    document_number TEXT NOT NULL,
    email           TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'ACTIVE',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_merchants_document_number UNIQUE (document_number)
);

CREATE TABLE payments (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id         UUID NOT NULL REFERENCES merchants (id),
    external_reference  TEXT NOT NULL,
    amount              NUMERIC(18, 2) NOT NULL CHECK (amount > 0),
    currency            TEXT NOT NULL DEFAULT 'COP',
    payment_method      TEXT NOT NULL,
    status              TEXT NOT NULL DEFAULT 'PENDING',
    idempotency_key     TEXT NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_payments_idempotency_key UNIQUE (idempotency_key),
    CONSTRAINT uq_payments_merchant_external_reference UNIQUE (merchant_id, external_reference)
);

CREATE INDEX idx_payments_merchant_id ON payments (merchant_id);
CREATE INDEX idx_payments_status ON payments (status);

CREATE TABLE payment_status_history (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    payment_id      UUID NOT NULL REFERENCES payments (id),
    previous_status TEXT,
    new_status      TEXT NOT NULL,
    reason          TEXT,
    changed_by      TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_payment_status_history_payment_id ON payment_status_history (payment_id);

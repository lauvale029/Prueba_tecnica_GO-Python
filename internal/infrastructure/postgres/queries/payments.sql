-- name: CreatePayment :one
INSERT INTO payments (
    id, merchant_id, external_reference, amount, currency,
    payment_method, status, idempotency_key, created_at, updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: GetPaymentByID :one
SELECT * FROM payments
WHERE id = $1;

-- name: GetPaymentByIdempotencyKey :one
SELECT * FROM payments
WHERE idempotency_key = $1;

-- name: GetPaymentByMerchantAndExternalReference :one
SELECT * FROM payments
WHERE merchant_id = $1 AND external_reference = $2;

-- name: UpdatePaymentStatus :one
UPDATE payments
SET status = $2, updated_at = $3
WHERE id = $1
RETURNING *;
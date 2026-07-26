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

-- name: ListPayments :many
SELECT * FROM payments
WHERE (sqlc.narg('merchant_id')::uuid IS NULL OR merchant_id = sqlc.narg('merchant_id'))
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status'))
  AND (sqlc.narg('payment_method')::text IS NULL OR payment_method = sqlc.narg('payment_method'))
  AND (sqlc.narg('date_from')::timestamptz IS NULL OR created_at >= sqlc.narg('date_from'))
  AND (sqlc.narg('date_to')::timestamptz IS NULL OR created_at <= sqlc.narg('date_to'))
ORDER BY created_at DESC
LIMIT sqlc.arg('page_limit') OFFSET sqlc.arg('page_offset');

-- name: CountPayments :one
SELECT count(*) FROM payments
WHERE (sqlc.narg('merchant_id')::uuid IS NULL OR merchant_id = sqlc.narg('merchant_id'))
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status'))
  AND (sqlc.narg('payment_method')::text IS NULL OR payment_method = sqlc.narg('payment_method'))
  AND (sqlc.narg('date_from')::timestamptz IS NULL OR created_at >= sqlc.narg('date_from'))
  AND (sqlc.narg('date_to')::timestamptz IS NULL OR created_at <= sqlc.narg('date_to'));
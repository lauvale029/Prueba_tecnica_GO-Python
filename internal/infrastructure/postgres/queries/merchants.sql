-- name: CreateMerchant :one
INSERT INTO merchants (id, name, document_number, email, status, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetMerchantByID :one
SELECT * FROM merchants
WHERE id = $1;
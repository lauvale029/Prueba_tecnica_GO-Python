-- name: CreatePaymentStatusHistory :one
INSERT INTO payment_status_history (id, payment_id, previous_status, new_status, reason, changed_by, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: ListPaymentStatusHistoryByPaymentID :many
SELECT * FROM payment_status_history
WHERE payment_id = $1
ORDER BY created_at ASC;
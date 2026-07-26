package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"

	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/application"
	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/domain"
	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/infrastructure/postgres/sqlcgen"
)

// PaymentRepository implementa application.PaymentRepository sobre las
// queries generadas por sqlc.
type PaymentRepository struct {
	queries *sqlcgen.Queries
}

func NewPaymentRepository(db *sql.DB) *PaymentRepository {
	return &PaymentRepository{queries: sqlcgen.New(db)}
}

var _ application.PaymentRepository = (*PaymentRepository)(nil)

func (r *PaymentRepository) Create(ctx context.Context, payment *domain.Payment) error {
	_, err := r.queries.CreatePayment(ctx, sqlcgen.CreatePaymentParams{
		ID:                payment.ID,
		MerchantID:        payment.MerchantID,
		ExternalReference: payment.ExternalReference,
		Amount:            payment.Amount.Amount,
		Currency:          payment.Amount.Currency,
		PaymentMethod:     string(payment.PaymentMethod),
		Status:            string(payment.Status),
		IdempotencyKey:    payment.IdempotencyKey,
		CreatedAt:         payment.CreatedAt,
		UpdatedAt:         payment.UpdatedAt,
	})
	return mapError(err)
}

func (r *PaymentRepository) GetByID(ctx context.Context, id string) (*domain.Payment, error) {
	row, err := r.queries.GetPaymentByID(ctx, id)
	if err != nil {
		return nil, mapError(err)
	}
	return toDomainPayment(row), nil
}

func (r *PaymentRepository) GetByIdempotencyKey(ctx context.Context, idempotencyKey string) (*domain.Payment, error) {
	row, err := r.queries.GetPaymentByIdempotencyKey(ctx, idempotencyKey)
	if err != nil {
		return nil, mapError(err)
	}
	return toDomainPayment(row), nil
}

func (r *PaymentRepository) GetByMerchantAndExternalReference(ctx context.Context, merchantID, externalReference string) (*domain.Payment, error) {
	row, err := r.queries.GetPaymentByMerchantAndExternalReference(ctx, sqlcgen.GetPaymentByMerchantAndExternalReferenceParams{
		MerchantID:        merchantID,
		ExternalReference: externalReference,
	})
	if err != nil {
		return nil, mapError(err)
	}
	return toDomainPayment(row), nil
}

func (r *PaymentRepository) UpdateStatus(ctx context.Context, payment *domain.Payment) error {
	_, err := r.queries.UpdatePaymentStatus(ctx, sqlcgen.UpdatePaymentStatusParams{
		ID:        payment.ID,
		Status:    string(payment.Status),
		UpdatedAt: payment.UpdatedAt,
	})
	return mapError(err)
}

func (r *PaymentRepository) List(ctx context.Context, filter application.PaymentFilter) ([]*domain.Payment, error) {
	merchantID, err := nullableUUID(filter.MerchantID)
	if err != nil {
		return nil, err
	}

	rows, err := r.queries.ListPayments(ctx, sqlcgen.ListPaymentsParams{
		MerchantID:    merchantID,
		Status:        nullableStatus(filter.Status),
		PaymentMethod: nullablePaymentMethod(filter.PaymentMethod),
		DateFrom:      nullableTime(filter.DateFrom),
		DateTo:        nullableTime(filter.DateTo),
		PageLimit:     int32(filter.Limit),
		PageOffset:    int32((filter.Page - 1) * filter.Limit),
	})
	if err != nil {
		return nil, mapError(err)
	}

	payments := make([]*domain.Payment, 0, len(rows))
	for _, row := range rows {
		payments = append(payments, toDomainPayment(row))
	}
	return payments, nil
}

func (r *PaymentRepository) Count(ctx context.Context, filter application.PaymentFilter) (int64, error) {
	merchantID, err := nullableUUID(filter.MerchantID)
	if err != nil {
		return 0, err
	}

	count, err := r.queries.CountPayments(ctx, sqlcgen.CountPaymentsParams{
		MerchantID:    merchantID,
		Status:        nullableStatus(filter.Status),
		PaymentMethod: nullablePaymentMethod(filter.PaymentMethod),
		DateFrom:      nullableTime(filter.DateFrom),
		DateTo:        nullableTime(filter.DateTo),
	})
	if err != nil {
		return 0, mapError(err)
	}
	return count, nil
}

// nullableUUID convierte un *string opcional a uuid.NullUUID, el tipo que
// sqlc genera para un parámetro de tipo uuid que puede ser NULL (ver
// ListPaymentsParams/CountPaymentsParams en sqlcgen).
func nullableUUID(id *string) (uuid.NullUUID, error) {
	if id == nil {
		return uuid.NullUUID{}, nil
	}
	parsed, err := uuid.Parse(*id)
	if err != nil {
		return uuid.NullUUID{}, err
	}
	return uuid.NullUUID{UUID: parsed, Valid: true}, nil
}

func nullableStatus(status *domain.PaymentStatus) sql.NullString {
	if status == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: string(*status), Valid: true}
}

func nullablePaymentMethod(method *domain.PaymentMethod) sql.NullString {
	if method == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: string(*method), Valid: true}
}

func nullableTime(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
}

// toDomainPayment reconstruye un domain.Payment a partir de una fila ya
// persistida. No pasa por domain.NewMoney/NewPayment a propósito: esos
// constructores validan reglas de negocio para datos *nuevos*, mientras
// que aquí solo estamos leyendo datos que ya fueron validados al crearse.
func toDomainPayment(row sqlcgen.Payment) *domain.Payment {
	return &domain.Payment{
		ID:                row.ID,
		MerchantID:        row.MerchantID,
		ExternalReference: row.ExternalReference,
		Amount: domain.Money{
			Amount:   row.Amount,
			Currency: row.Currency,
		},
		PaymentMethod:  domain.PaymentMethod(row.PaymentMethod),
		Status:         domain.PaymentStatus(row.Status),
		IdempotencyKey: row.IdempotencyKey,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}
}
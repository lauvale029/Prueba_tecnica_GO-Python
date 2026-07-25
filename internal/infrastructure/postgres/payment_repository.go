package postgres

import (
	"context"
	"database/sql"

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
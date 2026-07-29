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
// queries generadas por sqlc. Guarda *sql.DB en vez de un *sqlcgen.Queries
// ya armado porque cada método arma sus queries al momento (ver
// PaymentRepository.queries), para poder usar la transacción activa del
// contexto cuando la hay (ver tx.go/unit_of_work.go).
type PaymentRepository struct {
	db *sql.DB
}

func NewPaymentRepository(db *sql.DB) *PaymentRepository {
	return &PaymentRepository{db: db}
}

var _ application.PaymentRepository = (*PaymentRepository)(nil)

func (r *PaymentRepository) queries(ctx context.Context) *sqlcgen.Queries {
	return sqlcgen.New(dbtxFromContext(ctx, r.db))
}

func (r *PaymentRepository) Create(ctx context.Context, payment *domain.Payment) error {
	_, err := r.queries(ctx).CreatePayment(ctx, sqlcgen.CreatePaymentParams{
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
	row, err := r.queries(ctx).GetPaymentByID(ctx, id)
	if err != nil {
		return nil, mapError(err)
	}
	return toDomainPayment(row), nil
}

func (r *PaymentRepository) GetByIdempotencyKey(ctx context.Context, idempotencyKey string) (*domain.Payment, error) {
	row, err := r.queries(ctx).GetPaymentByIdempotencyKey(ctx, idempotencyKey)
	if err != nil {
		return nil, mapError(err)
	}
	return toDomainPayment(row), nil
}

func (r *PaymentRepository) GetByMerchantAndExternalReference(ctx context.Context, merchantID, externalReference string) (*domain.Payment, error) {
	row, err := r.queries(ctx).GetPaymentByMerchantAndExternalReference(ctx, sqlcgen.GetPaymentByMerchantAndExternalReferenceParams{
		MerchantID:        merchantID,
		ExternalReference: externalReference,
	})
	if err != nil {
		return nil, mapError(err)
	}
	return toDomainPayment(row), nil
}

func (r *PaymentRepository) UpdateStatus(ctx context.Context, payment *domain.Payment) error {
	_, err := r.queries(ctx).UpdatePaymentStatus(ctx, sqlcgen.UpdatePaymentStatusParams{
		ID:        payment.ID,
		Status:    string(payment.Status),
		UpdatedAt: payment.UpdatedAt,
	})
	return mapError(err)
}

// MarkProcessing guarda a qué proveedor se envió (providerName), la
// referencia de esa operación (providerReference), y el paso a
// PROCESSING, en una sola escritura (ver MarkPaymentProcessing en
// queries/payments.sql) — se persiste ANTES de llamar al proveedor
// externo, para que una caída a mitad de esa llamada deje todo lo
// necesario ya guardado para poder conciliar después.
func (r *PaymentRepository) MarkProcessing(ctx context.Context, paymentID, providerReference, providerName string, updatedAt time.Time) (*domain.Payment, error) {
	row, err := r.queries(ctx).MarkPaymentProcessing(ctx, sqlcgen.MarkPaymentProcessingParams{
		ID:                paymentID,
		ProviderReference: sql.NullString{String: providerReference, Valid: true},
		ProviderName:      sql.NullString{String: providerName, Valid: true},
		UpdatedAt:         updatedAt,
	})
	if err != nil {
		return nil, mapError(err)
	}
	return toDomainPayment(row), nil
}

func (r *PaymentRepository) List(ctx context.Context, filter application.PaymentFilter) ([]*domain.Payment, error) {
	merchantID, err := nullableUUID(filter.MerchantID)
	if err != nil {
		return nil, err
	}

	rows, err := r.queries(ctx).ListPayments(ctx, sqlcgen.ListPaymentsParams{
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

	count, err := r.queries(ctx).CountPayments(ctx, sqlcgen.CountPaymentsParams{
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

func (r *PaymentRepository) GetSummaryByMerchantID(ctx context.Context, merchantID string) (application.MerchantSummary, error) {
	row, err := r.queries(ctx).GetMerchantSummary(ctx, merchantID)
	if err != nil {
		return application.MerchantSummary{}, mapError(err)
	}

	return application.MerchantSummary{
		MerchantID:       merchantID,
		TotalPayments:    row.TotalPayments,
		ApprovedPayments: row.ApprovedPayments,
		RejectedPayments: row.RejectedPayments,
		PendingPayments:  row.PendingPayments,
		ApprovedAmount:   row.ApprovedAmount,
	}, nil
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
		PaymentMethod:     domain.PaymentMethod(row.PaymentMethod),
		Status:            domain.PaymentStatus(row.Status),
		IdempotencyKey:    row.IdempotencyKey,
		ProviderReference: fromNullString(row.ProviderReference),
		ProviderName:      fromNullString(row.ProviderName),
		CreatedAt:         row.CreatedAt,
		UpdatedAt:         row.UpdatedAt,
	}
}

func fromNullString(s sql.NullString) *string {
	if !s.Valid {
		return nil
	}
	return &s.String
}

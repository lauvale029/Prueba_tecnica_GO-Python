package postgres

import (
	"context"
	"database/sql"

	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/application"
	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/domain"
	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/infrastructure/postgres/sqlcgen"
)

// PaymentStatusHistoryRepository implementa
// application.PaymentStatusHistoryRepository sobre las queries generadas
// por sqlc.
type PaymentStatusHistoryRepository struct {
	queries *sqlcgen.Queries
}

func NewPaymentStatusHistoryRepository(db *sql.DB) *PaymentStatusHistoryRepository {
	return &PaymentStatusHistoryRepository{queries: sqlcgen.New(db)}
}

var _ application.PaymentStatusHistoryRepository = (*PaymentStatusHistoryRepository)(nil)

func (r *PaymentStatusHistoryRepository) Create(ctx context.Context, history *domain.PaymentStatusHistory) error {
	_, err := r.queries.CreatePaymentStatusHistory(ctx, sqlcgen.CreatePaymentStatusHistoryParams{
		ID:             history.ID,
		PaymentID:      history.PaymentID,
		PreviousStatus: toNullString(string(history.PreviousStatus)),
		NewStatus:      string(history.NewStatus),
		Reason:         toNullString(history.Reason),
		ChangedBy:      toNullString(history.ChangedBy),
		CreatedAt:      history.CreatedAt,
	})
	return mapError(err)
}

func (r *PaymentStatusHistoryRepository) ListByPaymentID(ctx context.Context, paymentID string) ([]*domain.PaymentStatusHistory, error) {
	rows, err := r.queries.ListPaymentStatusHistoryByPaymentID(ctx, paymentID)
	if err != nil {
		return nil, mapError(err)
	}

	history := make([]*domain.PaymentStatusHistory, 0, len(rows))
	for _, row := range rows {
		history = append(history, &domain.PaymentStatusHistory{
			ID:             row.ID,
			PaymentID:      row.PaymentID,
			PreviousStatus: domain.PaymentStatus(row.PreviousStatus.String),
			NewStatus:      domain.PaymentStatus(row.NewStatus),
			Reason:         row.Reason.String,
			ChangedBy:      row.ChangedBy.String,
			CreatedAt:      row.CreatedAt,
		})
	}
	return history, nil
}

// toNullString convierte un string de Go a sql.NullString, marcándolo
// como NULL cuando viene vacío (para columnas opcionales como reason o
// changed_by).
func toNullString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}
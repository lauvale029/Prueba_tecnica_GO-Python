package postgres

import (
	"context"
	"database/sql"

	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/application"
	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/domain"
	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/infrastructure/postgres/sqlcgen"
)

// MerchantRepository implementa application.MerchantRepository sobre
// las queries generadas por sqlc.
type MerchantRepository struct {
	queries *sqlcgen.Queries
}

func NewMerchantRepository(db *sql.DB) *MerchantRepository {
	return &MerchantRepository{queries: sqlcgen.New(db)}
}

var _ application.MerchantRepository = (*MerchantRepository)(nil)

func (r *MerchantRepository) Create(ctx context.Context, merchant *domain.Merchant) error {
	_, err := r.queries.CreateMerchant(ctx, sqlcgen.CreateMerchantParams{
		ID:             merchant.ID,
		Name:           merchant.Name,
		DocumentNumber: merchant.DocumentNumber,
		Email:          merchant.Email,
		Status:         string(merchant.Status),
		CreatedAt:      merchant.CreatedAt,
		UpdatedAt:      merchant.UpdatedAt,
	})
	return mapError(err)
}

func (r *MerchantRepository) GetByID(ctx context.Context, id string) (*domain.Merchant, error) {
	row, err := r.queries.GetMerchantByID(ctx, id)
	if err != nil {
		return nil, mapError(err)
	}
	return toDomainMerchant(row), nil
}

func toDomainMerchant(row sqlcgen.Merchant) *domain.Merchant {
	return &domain.Merchant{
		ID:             row.ID,
		Name:           row.Name,
		DocumentNumber: row.DocumentNumber,
		Email:          row.Email,
		Status:         domain.MerchantStatus(row.Status),
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}
}
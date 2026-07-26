package application

import (
	"context"

	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/domain"
)

// MerchantService orquesta el caso de uso de comercios: valida con el
// dominio y persiste a través del puerto MerchantRepository, sin conocer
// la implementación concreta (Postgres u otra).
type MerchantService struct {
	repo MerchantRepository
}

func NewMerchantService(repo MerchantRepository) *MerchantService {
	return &MerchantService{repo: repo}
}

// Create valida y crea un comercio nuevo.
func (s *MerchantService) Create(ctx context.Context, name, documentNumber, email string) (*domain.Merchant, error) {
	merchant, err := domain.NewMerchant(name, documentNumber, email)
	if err != nil {
		return nil, err
	}

	if err := s.repo.Create(ctx, merchant); err != nil {
		return nil, err
	}

	return merchant, nil
}

// Get consulta un comercio existente por su ID.
func (s *MerchantService) Get(ctx context.Context, id string) (*domain.Merchant, error) {
	return s.repo.GetByID(ctx, id)
}
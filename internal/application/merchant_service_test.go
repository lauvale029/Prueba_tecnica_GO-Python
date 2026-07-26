package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/application"
	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/domain"
)

// fakeMerchantRepository es un doble de prueba en memoria. No reutiliza
// los errores de internal/infrastructure/postgres a propósito: application
// no debe depender de infrastructure, ni siquiera en sus tests.
type fakeMerchantRepository struct {
	createErr error
	getErr    error
	stored    *domain.Merchant
}

func (f *fakeMerchantRepository) Create(_ context.Context, merchant *domain.Merchant) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.stored = merchant
	return nil
}

func (f *fakeMerchantRepository) GetByID(_ context.Context, id string) (*domain.Merchant, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.stored, nil
}

func TestMerchantService_Create_Valid(t *testing.T) {
	repo := &fakeMerchantRepository{}
	service := application.NewMerchantService(repo)

	merchant, err := service.Create(context.Background(), "Comercio Prueba", "900123456", "comercio@example.com")

	require.NoError(t, err)
	require.NotEmpty(t, merchant.ID)
	require.Equal(t, merchant, repo.stored)
}

func TestMerchantService_Create_InvalidEmail(t *testing.T) {
	repo := &fakeMerchantRepository{}
	service := application.NewMerchantService(repo)

	_, err := service.Create(context.Background(), "Comercio Prueba", "900123456", "no-es-un-email")

	require.ErrorIs(t, err, domain.ErrInvalidEmail)
	require.Nil(t, repo.stored, "el dominio debe rechazar antes de llegar al repositorio")
}

func TestMerchantService_Create_RepositoryError(t *testing.T) {
	wantErr := errors.New("fallo simulado del repositorio")
	repo := &fakeMerchantRepository{createErr: wantErr}
	service := application.NewMerchantService(repo)

	_, err := service.Create(context.Background(), "Comercio Prueba", "900123456", "comercio@example.com")

	require.ErrorIs(t, err, wantErr)
}

func TestMerchantService_Get(t *testing.T) {
	existing, err := domain.NewMerchant("Comercio Prueba", "900123456", "comercio@example.com")
	require.NoError(t, err)
	repo := &fakeMerchantRepository{stored: existing}
	service := application.NewMerchantService(repo)

	found, err := service.Get(context.Background(), existing.ID)

	require.NoError(t, err)
	require.Equal(t, existing, found)
}

func TestMerchantService_Get_NotFound(t *testing.T) {
	wantErr := errors.New("no existe, cualquier error sirve para esta prueba")
	repo := &fakeMerchantRepository{getErr: wantErr}
	service := application.NewMerchantService(repo)

	_, err := service.Get(context.Background(), "id-inventado")

	require.ErrorIs(t, err, wantErr)
}
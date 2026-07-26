//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/application"
	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/domain"
	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/infrastructure/postgres"
)

func TestMerchantRepository_CreateAndGetByID(t *testing.T) {
	db := testDB(t)
	repo := postgres.NewMerchantRepository(db)
	ctx := context.Background()

	merchant := createTestMerchant(t, db)

	found, err := repo.GetByID(ctx, merchant.ID)
	require.NoError(t, err)
	require.Equal(t, merchant.Name, found.Name)
	require.Equal(t, merchant.DocumentNumber, found.DocumentNumber)
	require.Equal(t, merchant.Email, found.Email)
	require.Equal(t, merchant.Status, found.Status)
}

func TestMerchantRepository_GetByID_NotFound(t *testing.T) {
	db := testDB(t)
	repo := postgres.NewMerchantRepository(db)

	_, err := repo.GetByID(context.Background(), uuid.New().String())
	require.ErrorIs(t, err, application.ErrNotFound)
}

func TestMerchantRepository_Create_DuplicateDocumentNumber(t *testing.T) {
	db := testDB(t)
	repo := postgres.NewMerchantRepository(db)
	ctx := context.Background()

	first := createTestMerchant(t, db)

	second, err := domain.NewMerchant("Otro Comercio", first.DocumentNumber, "otro@example.com")
	require.NoError(t, err)

	err = repo.Create(ctx, second)
	require.ErrorIs(t, err, application.ErrConflict)
}
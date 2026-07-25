//go:build integration

package postgres_test

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/domain"
	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/infrastructure/postgres"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL no está definido; exporta las variables de .env antes de correr los tests de integración")
	}
	db, err := postgres.NewDB(url)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

func createTestMerchant(t *testing.T, db *sql.DB) *domain.Merchant {
	t.Helper()
	repo := postgres.NewMerchantRepository(db)
	merchant, err := domain.NewMerchant("Comercio Integración", "doc-"+uuid.New().String(), "integracion@example.com")
	require.NoError(t, err)
	require.NoError(t, repo.Create(context.Background(), merchant))
	return merchant
}
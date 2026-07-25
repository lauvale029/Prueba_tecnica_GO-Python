//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/domain"
	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/infrastructure/postgres"
)

func newTestPayment(t *testing.T, merchantID, idempotencyKey string) *domain.Payment {
	t.Helper()
	payment, err := domain.NewPayment(merchantID, "ORDER-"+uuid.New().String(), decimal.NewFromInt(1000), "COP", domain.PaymentMethodQR, idempotencyKey)
	require.NoError(t, err)
	return payment
}

func TestPaymentRepository_CreateAndGetByID(t *testing.T) {
	db := testDB(t)
	merchant := createTestMerchant(t, db)
	repo := postgres.NewPaymentRepository(db)
	ctx := context.Background()

	payment := newTestPayment(t, merchant.ID, "key-"+uuid.New().String())
	require.NoError(t, repo.Create(ctx, payment))

	found, err := repo.GetByID(ctx, payment.ID)
	require.NoError(t, err)
	require.True(t, payment.Amount.Amount.Equal(found.Amount.Amount))
	require.Equal(t, payment.Amount.Currency, found.Amount.Currency)
	require.Equal(t, payment.Status, found.Status)
}

func TestPaymentRepository_GetByIdempotencyKey(t *testing.T) {
	db := testDB(t)
	merchant := createTestMerchant(t, db)
	repo := postgres.NewPaymentRepository(db)
	ctx := context.Background()

	key := "key-" + uuid.New().String()
	payment := newTestPayment(t, merchant.ID, key)
	require.NoError(t, repo.Create(ctx, payment))

	found, err := repo.GetByIdempotencyKey(ctx, key)
	require.NoError(t, err)
	require.Equal(t, payment.ID, found.ID)
}

func TestPaymentRepository_GetByMerchantAndExternalReference(t *testing.T) {
	db := testDB(t)
	merchant := createTestMerchant(t, db)
	repo := postgres.NewPaymentRepository(db)
	ctx := context.Background()

	payment := newTestPayment(t, merchant.ID, "key-"+uuid.New().String())
	require.NoError(t, repo.Create(ctx, payment))

	found, err := repo.GetByMerchantAndExternalReference(ctx, merchant.ID, payment.ExternalReference)
	require.NoError(t, err)
	require.Equal(t, payment.ID, found.ID)
}

func TestPaymentRepository_Create_DuplicateIdempotencyKey(t *testing.T) {
	db := testDB(t)
	merchant := createTestMerchant(t, db)
	repo := postgres.NewPaymentRepository(db)
	ctx := context.Background()

	key := "key-" + uuid.New().String()
	first := newTestPayment(t, merchant.ID, key)
	require.NoError(t, repo.Create(ctx, first))

	second := newTestPayment(t, merchant.ID, key)
	err := repo.Create(ctx, second)
	require.ErrorIs(t, err, postgres.ErrConflict)
}

func TestPaymentRepository_Create_DuplicateExternalReference(t *testing.T) {
	db := testDB(t)
	merchant := createTestMerchant(t, db)
	repo := postgres.NewPaymentRepository(db)
	ctx := context.Background()

	first := newTestPayment(t, merchant.ID, "key-"+uuid.New().String())
	require.NoError(t, repo.Create(ctx, first))

	second, err := domain.NewPayment(merchant.ID, first.ExternalReference, decimal.NewFromInt(2000), "COP", domain.PaymentMethodCard, "key-"+uuid.New().String())
	require.NoError(t, err)

	err = repo.Create(ctx, second)
	require.ErrorIs(t, err, postgres.ErrConflict)
}

func TestPaymentRepository_UpdateStatus(t *testing.T) {
	db := testDB(t)
	merchant := createTestMerchant(t, db)
	repo := postgres.NewPaymentRepository(db)
	ctx := context.Background()

	payment := newTestPayment(t, merchant.ID, "key-"+uuid.New().String())
	require.NoError(t, repo.Create(ctx, payment))

	require.NoError(t, payment.ChangeStatus(domain.PaymentStatusApproved))
	require.NoError(t, repo.UpdateStatus(ctx, payment))

	found, err := repo.GetByID(ctx, payment.ID)
	require.NoError(t, err)
	require.Equal(t, domain.PaymentStatusApproved, found.Status)
}
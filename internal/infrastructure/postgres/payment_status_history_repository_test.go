//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/domain"
	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/infrastructure/postgres"
)

func TestPaymentStatusHistoryRepository_CreateAndList(t *testing.T) {
	db := testDB(t)
	merchant := createTestMerchant(t, db)
	paymentRepo := postgres.NewPaymentRepository(db)
	historyRepo := postgres.NewPaymentStatusHistoryRepository(db)
	ctx := context.Background()

	payment := newTestPayment(t, merchant.ID, "key-"+uuid.New().String())
	require.NoError(t, paymentRepo.Create(ctx, payment))

	history := domain.NewPaymentStatusHistory(payment.ID, domain.PaymentStatusPending, domain.PaymentStatusApproved, "pago confirmado", "test-user")
	require.NoError(t, historyRepo.Create(ctx, history))

	entries, err := historyRepo.ListByPaymentID(ctx, payment.ID)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, domain.PaymentStatusPending, entries[0].PreviousStatus)
	require.Equal(t, domain.PaymentStatusApproved, entries[0].NewStatus)
	require.Equal(t, "pago confirmado", entries[0].Reason)
	require.Equal(t, "test-user", entries[0].ChangedBy)
}
//go:build integration

package postgres_test

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/application"
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
	require.ErrorIs(t, err, application.ErrConflict)
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
	require.ErrorIs(t, err, application.ErrConflict)
}

// TestPaymentRepository_Create_ConcurrentSameIdempotencyKey es la prueba
// definitiva de la restricción única: a diferencia del test de
// concurrencia en internal/application (que simula la atomicidad con un
// mutex), acá 20 goroutines de verdad golpean Postgres al mismo tiempo
// con la misma idempotency_key. Sin la restricción UNIQUE, esto podría
// crear más de una fila; con ella, es imposible.
func TestPaymentRepository_Create_ConcurrentSameIdempotencyKey(t *testing.T) {
	db := testDB(t)
	merchant := createTestMerchant(t, db)
	repo := postgres.NewPaymentRepository(db)

	const attempts = 20
	key := "key-" + uuid.New().String()

	var wg sync.WaitGroup
	errs := make([]error, attempts)

	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			payment, err := domain.NewPayment(merchant.ID, "ORDER-"+uuid.New().String(), decimal.NewFromInt(1000), "COP", domain.PaymentMethodQR, key)
			if err != nil {
				errs[i] = err
				return
			}
			errs[i] = repo.Create(context.Background(), payment)
		}(i)
	}
	wg.Wait()

	successes := 0
	for _, err := range errs {
		if err == nil {
			successes++
			continue
		}
		require.ErrorIs(t, err, application.ErrConflict, "cualquier fallo debe ser por la restricción única, no otro error")
	}

	require.Equal(t, 1, successes, "exactamente una inserción concurrente debe tener éxito")

	found, err := repo.GetByIdempotencyKey(context.Background(), key)
	require.NoError(t, err)
	require.NotNil(t, found)
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
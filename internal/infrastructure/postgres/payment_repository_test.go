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
func TestPaymentRepository_ListAndCount_FilterByMerchant(t *testing.T) {
	db := testDB(t)
	merchantA := createTestMerchant(t, db)
	merchantB := createTestMerchant(t, db)
	repo := postgres.NewPaymentRepository(db)
	ctx := context.Background()

	require.NoError(t, repo.Create(ctx, newTestPayment(t, merchantA.ID, "key-"+uuid.New().String())))
	require.NoError(t, repo.Create(ctx, newTestPayment(t, merchantB.ID, "key-"+uuid.New().String())))

	filter := application.PaymentFilter{MerchantID: &merchantA.ID, Page: 1, Limit: 10}

	results, err := repo.List(ctx, filter)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, merchantA.ID, results[0].MerchantID)

	total, err := repo.Count(ctx, filter)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
}

func TestPaymentRepository_ListAndCount_FilterByStatus(t *testing.T) {
	db := testDB(t)
	merchant := createTestMerchant(t, db)
	repo := postgres.NewPaymentRepository(db)
	ctx := context.Background()

	pending := newTestPayment(t, merchant.ID, "key-"+uuid.New().String())
	require.NoError(t, repo.Create(ctx, pending))

	approved := newTestPayment(t, merchant.ID, "key-"+uuid.New().String())
	require.NoError(t, repo.Create(ctx, approved))
	require.NoError(t, approved.ChangeStatus(domain.PaymentStatusApproved))
	require.NoError(t, repo.UpdateStatus(ctx, approved))

	status := domain.PaymentStatusApproved
	filter := application.PaymentFilter{MerchantID: &merchant.ID, Status: &status, Page: 1, Limit: 10}

	results, err := repo.List(ctx, filter)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, approved.ID, results[0].ID)

	total, err := repo.Count(ctx, filter)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
}

func TestPaymentRepository_List_Pagination(t *testing.T) {
	db := testDB(t)
	merchant := createTestMerchant(t, db)
	repo := postgres.NewPaymentRepository(db)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		require.NoError(t, repo.Create(ctx, newTestPayment(t, merchant.ID, "key-"+uuid.New().String())))
	}

	filter := application.PaymentFilter{MerchantID: &merchant.ID, Page: 1, Limit: 2}
	firstPage, err := repo.List(ctx, filter)
	require.NoError(t, err)
	require.Len(t, firstPage, 2)

	filter.Page = 2
	secondPage, err := repo.List(ctx, filter)
	require.NoError(t, err)
	require.Len(t, secondPage, 1)

	total, err := repo.Count(ctx, application.PaymentFilter{MerchantID: &merchant.ID})
	require.NoError(t, err)
	require.Equal(t, int64(3), total)
}

func TestPaymentRepository_GetSummaryByMerchantID(t *testing.T) {
	db := testDB(t)
	merchant := createTestMerchant(t, db)
	repo := postgres.NewPaymentRepository(db)
	ctx := context.Background()

	approved := newTestPayment(t, merchant.ID, "key-"+uuid.New().String())
	require.NoError(t, repo.Create(ctx, approved))
	require.NoError(t, approved.ChangeStatus(domain.PaymentStatusApproved))
	require.NoError(t, repo.UpdateStatus(ctx, approved))

	rejected := newTestPayment(t, merchant.ID, "key-"+uuid.New().String())
	require.NoError(t, repo.Create(ctx, rejected))
	require.NoError(t, rejected.ChangeStatus(domain.PaymentStatusRejected))
	require.NoError(t, repo.UpdateStatus(ctx, rejected))

	require.NoError(t, repo.Create(ctx, newTestPayment(t, merchant.ID, "key-"+uuid.New().String())))

	summary, err := repo.GetSummaryByMerchantID(ctx, merchant.ID)
	require.NoError(t, err)

	require.Equal(t, int64(3), summary.TotalPayments)
	require.Equal(t, int64(1), summary.ApprovedPayments)
	require.Equal(t, int64(1), summary.RejectedPayments)
	require.Equal(t, int64(1), summary.PendingPayments)
	require.True(t, approved.Amount.Amount.Equal(summary.ApprovedAmount))
}

func TestPaymentRepository_GetSummaryByMerchantID_NoPayments(t *testing.T) {
	db := testDB(t)
	merchant := createTestMerchant(t, db)
	repo := postgres.NewPaymentRepository(db)

	summary, err := repo.GetSummaryByMerchantID(context.Background(), merchant.ID)

	require.NoError(t, err)
	require.Equal(t, int64(0), summary.TotalPayments)
	require.True(t, summary.ApprovedAmount.IsZero())
}

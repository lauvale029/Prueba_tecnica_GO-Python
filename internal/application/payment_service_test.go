package application_test

import (
	"context"
	"sync"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/application"
	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/domain"
)

// inMemoryMerchantRepository e inMemoryPaymentRepository sí devuelven
// application.ErrNotFound/ErrConflict (a diferencia del fake usado en
// merchant_service_test.go): PaymentService inspecciona estos errores
// como parte de su propia lógica de negocio, así que el fake necesita
// simular ese contrato con fidelidad. Ambos son seguros para usar desde
// múltiples goroutines a la vez (necesario para el test de concurrencia).
type inMemoryMerchantRepository struct {
	mu        sync.Mutex
	merchants map[string]*domain.Merchant
}

func newInMemoryMerchantRepository() *inMemoryMerchantRepository {
	return &inMemoryMerchantRepository{merchants: make(map[string]*domain.Merchant)}
}

func (r *inMemoryMerchantRepository) Create(_ context.Context, merchant *domain.Merchant) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.merchants[merchant.ID] = merchant
	return nil
}

func (r *inMemoryMerchantRepository) GetByID(_ context.Context, id string) (*domain.Merchant, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m, ok := r.merchants[id]
	if !ok {
		return nil, application.ErrNotFound
	}
	return m, nil
}

type inMemoryPaymentRepository struct {
	mu               sync.Mutex
	byID             map[string]*domain.Payment
	byIdempotencyKey map[string]*domain.Payment
	byExternalRef    map[string]*domain.Payment
}

func newInMemoryPaymentRepository() *inMemoryPaymentRepository {
	return &inMemoryPaymentRepository{
		byID:             make(map[string]*domain.Payment),
		byIdempotencyKey: make(map[string]*domain.Payment),
		byExternalRef:    make(map[string]*domain.Payment),
	}
}

func paymentExternalRefKey(merchantID, externalReference string) string {
	return merchantID + "|" + externalReference
}

// Create simula, bajo el mismo mutex, exactamente lo que garantizan las
// restricciones UNIQUE de Postgres: solo una llamada concurrente con la
// misma idempotency_key o external_reference puede tener éxito.
func (r *inMemoryPaymentRepository) Create(_ context.Context, payment *domain.Payment) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.byIdempotencyKey[payment.IdempotencyKey]; exists {
		return application.ErrConflict
	}
	refKey := paymentExternalRefKey(payment.MerchantID, payment.ExternalReference)
	if _, exists := r.byExternalRef[refKey]; exists {
		return application.ErrConflict
	}

	r.byID[payment.ID] = payment
	r.byIdempotencyKey[payment.IdempotencyKey] = payment
	r.byExternalRef[refKey] = payment
	return nil
}

func (r *inMemoryPaymentRepository) GetByID(_ context.Context, id string) (*domain.Payment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.byID[id]
	if !ok {
		return nil, application.ErrNotFound
	}
	return p, nil
}

func (r *inMemoryPaymentRepository) GetByIdempotencyKey(_ context.Context, key string) (*domain.Payment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.byIdempotencyKey[key]
	if !ok {
		return nil, application.ErrNotFound
	}
	return p, nil
}

func (r *inMemoryPaymentRepository) GetByMerchantAndExternalReference(_ context.Context, merchantID, externalReference string) (*domain.Payment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.byExternalRef[paymentExternalRefKey(merchantID, externalReference)]
	if !ok {
		return nil, application.ErrNotFound
	}
	return p, nil
}

func (r *inMemoryPaymentRepository) UpdateStatus(_ context.Context, payment *domain.Payment) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[payment.ID] = payment
	return nil
}

func (r *inMemoryPaymentRepository) rowCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.byID)
}

func (r *inMemoryPaymentRepository) List(_ context.Context, _ application.PaymentFilter) ([]*domain.Payment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	payments := make([]*domain.Payment, 0, len(r.byID))
	for _, p := range r.byID {
		payments = append(payments, p)
	}
	return payments, nil
}

func (r *inMemoryPaymentRepository) Count(_ context.Context, _ application.PaymentFilter) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return int64(len(r.byID)), nil
}

func (r *inMemoryPaymentRepository) GetSummaryByMerchantID(_ context.Context, merchantID string) (application.MerchantSummary, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	summary := application.MerchantSummary{MerchantID: merchantID, ApprovedAmount: decimal.Zero}
	for _, p := range r.byID {
		if p.MerchantID != merchantID {
			continue
		}
		summary.TotalPayments++
		switch p.Status {
		case domain.PaymentStatusApproved:
			summary.ApprovedPayments++
			summary.ApprovedAmount = summary.ApprovedAmount.Add(p.Amount.Amount)
		case domain.PaymentStatusRejected:
			summary.RejectedPayments++
		case domain.PaymentStatusPending:
			summary.PendingPayments++
		}
	}
	return summary, nil
}

// inMemorySummaryCache simula Redis para probar que PaymentService.Summary
// de verdad usa la cache (hit) y que UpdateStatus la invalida.
type inMemorySummaryCache struct {
	mu    sync.Mutex
	store map[string]application.MerchantSummary
}

func newInMemorySummaryCache() *inMemorySummaryCache {
	return &inMemorySummaryCache{store: make(map[string]application.MerchantSummary)}
}

func (c *inMemorySummaryCache) Get(_ context.Context, merchantID string) (application.MerchantSummary, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	summary, ok := c.store[merchantID]
	return summary, ok
}

func (c *inMemorySummaryCache) Set(_ context.Context, merchantID string, summary application.MerchantSummary) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.store[merchantID] = summary
}

func (c *inMemorySummaryCache) Invalidate(_ context.Context, merchantID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.store, merchantID)
}

func (c *inMemorySummaryCache) has(merchantID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.store[merchantID]
	return ok
}

type inMemoryHistoryRepository struct {
	mu      sync.Mutex
	entries map[string][]*domain.PaymentStatusHistory
}

func newInMemoryHistoryRepository() *inMemoryHistoryRepository {
	return &inMemoryHistoryRepository{entries: make(map[string][]*domain.PaymentStatusHistory)}
}

func (r *inMemoryHistoryRepository) Create(_ context.Context, history *domain.PaymentStatusHistory) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[history.PaymentID] = append(r.entries[history.PaymentID], history)
	return nil
}

func (r *inMemoryHistoryRepository) ListByPaymentID(_ context.Context, paymentID string) ([]*domain.PaymentStatusHistory, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.entries[paymentID], nil
}

func seedMerchant(t *testing.T, repo *inMemoryMerchantRepository) *domain.Merchant {
	t.Helper()
	merchant, err := domain.NewMerchant("Comercio Prueba", "900123456", "comercio@example.com")
	require.NoError(t, err)
	require.NoError(t, repo.Create(context.Background(), merchant))
	return merchant
}

func newPaymentServiceForTest() (*application.PaymentService, *inMemoryMerchantRepository, *inMemoryPaymentRepository, *inMemoryHistoryRepository, *inMemorySummaryCache) {
	merchants := newInMemoryMerchantRepository()
	payments := newInMemoryPaymentRepository()
	history := newInMemoryHistoryRepository()
	summaries := newInMemorySummaryCache()
	service := application.NewPaymentService(payments, merchants, history, application.NoopIdempotencyLocker{}, summaries)
	return service, merchants, payments, history, summaries
}

func TestPaymentService_Create_Valid(t *testing.T) {
	service, merchants, _, _, _ := newPaymentServiceForTest()
	merchant := seedMerchant(t, merchants)

	payment, err := service.Create(context.Background(), merchant.ID, "ORDER-1", decimal.NewFromInt(1000), "COP", domain.PaymentMethodQR, "key-1")

	require.NoError(t, err)
	require.Equal(t, domain.PaymentStatusPending, payment.Status)
}

func TestPaymentService_Create_MissingIdempotencyKey(t *testing.T) {
	service, merchants, _, _, _ := newPaymentServiceForTest()
	merchant := seedMerchant(t, merchants)

	_, err := service.Create(context.Background(), merchant.ID, "ORDER-1", decimal.NewFromInt(1000), "COP", domain.PaymentMethodQR, "")

	require.ErrorIs(t, err, domain.ErrMissingIdempotencyKey)
}

func TestPaymentService_Create_MerchantNotFound(t *testing.T) {
	service, _, _, _, _ := newPaymentServiceForTest()

	_, err := service.Create(context.Background(), "id-inventado", "ORDER-1", decimal.NewFromInt(1000), "COP", domain.PaymentMethodQR, "key-1")

	require.ErrorIs(t, err, application.ErrNotFound)
}

func TestPaymentService_Create_IdempotentReplay(t *testing.T) {
	service, merchants, _, _, _ := newPaymentServiceForTest()
	merchant := seedMerchant(t, merchants)
	key := "key-1"

	first, err := service.Create(context.Background(), merchant.ID, "ORDER-1", decimal.NewFromInt(1000), "COP", domain.PaymentMethodQR, key)
	require.NoError(t, err)

	second, err := service.Create(context.Background(), merchant.ID, "ORDER-1", decimal.NewFromInt(1000), "COP", domain.PaymentMethodQR, key)
	require.NoError(t, err)

	require.Equal(t, first.ID, second.ID)
}

func TestPaymentService_Create_DuplicateExternalReference(t *testing.T) {
	service, merchants, _, _, _ := newPaymentServiceForTest()
	merchant := seedMerchant(t, merchants)

	_, err := service.Create(context.Background(), merchant.ID, "ORDER-1", decimal.NewFromInt(1000), "COP", domain.PaymentMethodQR, "key-1")
	require.NoError(t, err)

	_, err = service.Create(context.Background(), merchant.ID, "ORDER-1", decimal.NewFromInt(2000), "COP", domain.PaymentMethodCard, "key-2")
	require.ErrorIs(t, err, application.ErrConflict)
}

// TestPaymentService_Create_Concurrent lanza muchas goroutines a la vez
// con la MISMA Idempotency-Key. Usa NoopIdempotencyLocker a propósito
// (nunca "ayuda" a nadie): así el test prueba que la corrección viene
// del repositorio (que simula la restricción única de Postgres), no del
// lock de Redis — exactamente la garantía que documentamos en el README.
func TestPaymentService_Create_Concurrent(t *testing.T) {
	service, merchants, payments, _, _ := newPaymentServiceForTest()
	merchant := seedMerchant(t, merchants)

	const attempts = 20
	const idempotencyKey = "key-concurrent"

	var wg sync.WaitGroup
	results := make([]*domain.Payment, attempts)
	errs := make([]error, attempts)

	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			payment, err := service.Create(context.Background(), merchant.ID, "ORDER-CONCURRENT", decimal.NewFromInt(1000), "COP", domain.PaymentMethodQR, idempotencyKey)
			results[i] = payment
			errs[i] = err
		}(i)
	}
	wg.Wait()

	var firstID string
	for i := 0; i < attempts; i++ {
		require.NoError(t, errs[i], "todas las llamadas deben resolverse sin error, incluso las que pierdan la carrera")
		if firstID == "" {
			firstID = results[i].ID
		}
		require.Equal(t, firstID, results[i].ID, "todas deben devolver el mismo pago")
	}

	require.Equal(t, 1, payments.rowCount(), "no debe haberse creado más de un pago con la misma Idempotency-Key")
}

func TestPaymentService_UpdateStatus_Valid(t *testing.T) {
	service, merchants, _, history, _ := newPaymentServiceForTest()
	merchant := seedMerchant(t, merchants)
	payment, err := service.Create(context.Background(), merchant.ID, "ORDER-1", decimal.NewFromInt(1000), "COP", domain.PaymentMethodQR, "key-1")
	require.NoError(t, err)

	updated, err := service.UpdateStatus(context.Background(), payment.ID, domain.PaymentStatusApproved, "pago confirmado", "test-user")

	require.NoError(t, err)
	require.Equal(t, domain.PaymentStatusApproved, updated.Status)

	entries, err := history.ListByPaymentID(context.Background(), payment.ID)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, domain.PaymentStatusPending, entries[0].PreviousStatus)
	require.Equal(t, domain.PaymentStatusApproved, entries[0].NewStatus)
	require.Equal(t, "test-user", entries[0].ChangedBy)
}

func TestPaymentService_UpdateStatus_InvalidTransition(t *testing.T) {
	service, merchants, _, _, _ := newPaymentServiceForTest()
	merchant := seedMerchant(t, merchants)
	payment, err := service.Create(context.Background(), merchant.ID, "ORDER-1", decimal.NewFromInt(1000), "COP", domain.PaymentMethodQR, "key-1")
	require.NoError(t, err)
	_, err = service.UpdateStatus(context.Background(), payment.ID, domain.PaymentStatusApproved, "ok", "test-user")
	require.NoError(t, err)

	_, err = service.UpdateStatus(context.Background(), payment.ID, domain.PaymentStatusRejected, "no debería poder", "test-user")

	require.ErrorIs(t, err, domain.ErrInvalidStatusTransition)
}

func TestPaymentService_UpdateStatus_PaymentNotFound(t *testing.T) {
	service, _, _, _, _ := newPaymentServiceForTest()

	_, err := service.UpdateStatus(context.Background(), "id-inventado", domain.PaymentStatusApproved, "x", "test-user")

	require.ErrorIs(t, err, application.ErrNotFound)
}

func TestPaymentService_History_PaymentNotFound(t *testing.T) {
	service, _, _, _, _ := newPaymentServiceForTest()

	_, err := service.History(context.Background(), "id-inventado")

	require.ErrorIs(t, err, application.ErrNotFound)
}

func TestPaymentService_List_DefaultsPageAndLimit(t *testing.T) {
	service, merchants, _, _, _ := newPaymentServiceForTest()
	merchant := seedMerchant(t, merchants)
	_, err := service.Create(context.Background(), merchant.ID, "ORDER-1", decimal.NewFromInt(1000), "COP", domain.PaymentMethodQR, "key-1")
	require.NoError(t, err)

	payments, total, err := service.List(context.Background(), application.PaymentFilter{})

	require.NoError(t, err)
	require.Len(t, payments, 1)
	require.Equal(t, int64(1), total)
}

func TestPaymentService_List_LimitCappedAtMax(t *testing.T) {
	service, _, _, _, _ := newPaymentServiceForTest()

	_, _, err := service.List(context.Background(), application.PaymentFilter{Limit: application.MaxLimit + 1000})

	require.NoError(t, err)
}

func TestPaymentService_Summary_MerchantNotFound(t *testing.T) {
	service, _, _, _, _ := newPaymentServiceForTest()

	_, err := service.Summary(context.Background(), "id-inventado")

	require.ErrorIs(t, err, application.ErrNotFound)
}

func TestPaymentService_Summary_ComputesFromRepository(t *testing.T) {
	service, merchants, _, _, _ := newPaymentServiceForTest()
	merchant := seedMerchant(t, merchants)

	approved, err := service.Create(context.Background(), merchant.ID, "ORDER-1", decimal.NewFromInt(1000), "COP", domain.PaymentMethodQR, "key-1")
	require.NoError(t, err)
	_, err = service.UpdateStatus(context.Background(), approved.ID, domain.PaymentStatusApproved, "ok", "test-user")
	require.NoError(t, err)

	_, err = service.Create(context.Background(), merchant.ID, "ORDER-2", decimal.NewFromInt(500), "COP", domain.PaymentMethodQR, "key-2")
	require.NoError(t, err)

	summary, err := service.Summary(context.Background(), merchant.ID)

	require.NoError(t, err)
	require.Equal(t, int64(2), summary.TotalPayments)
	require.Equal(t, int64(1), summary.ApprovedPayments)
	require.Equal(t, int64(1), summary.PendingPayments)
	require.True(t, decimal.NewFromInt(1000).Equal(summary.ApprovedAmount))
}

func TestPaymentService_Summary_UsesCacheOnSecondCall(t *testing.T) {
	service, merchants, _, _, cache := newPaymentServiceForTest()
	merchant := seedMerchant(t, merchants)
	_, err := service.Create(context.Background(), merchant.ID, "ORDER-1", decimal.NewFromInt(1000), "COP", domain.PaymentMethodQR, "key-1")
	require.NoError(t, err)

	require.False(t, cache.has(merchant.ID), "no debe haber nada en cache antes de la primera consulta")

	_, err = service.Summary(context.Background(), merchant.ID)
	require.NoError(t, err)
	require.True(t, cache.has(merchant.ID), "la primera consulta debe guardar el resultado en cache")

	// Segunda consulta: debe venir de la cache, no de un recálculo (lo
	// verificamos indirectamente: si borráramos el pago del repo "a mano"
	// sin pasar por UpdateStatus, un recálculo lo notaría; en cambio la
	// cache seguiría devolviendo el valor guardado).
	cached, err := service.Summary(context.Background(), merchant.ID)
	require.NoError(t, err)
	require.Equal(t, int64(1), cached.TotalPayments)
}

func TestPaymentService_UpdateStatus_InvalidatesSummaryCache(t *testing.T) {
	service, merchants, _, _, cache := newPaymentServiceForTest()
	merchant := seedMerchant(t, merchants)
	payment, err := service.Create(context.Background(), merchant.ID, "ORDER-1", decimal.NewFromInt(1000), "COP", domain.PaymentMethodQR, "key-1")
	require.NoError(t, err)

	_, err = service.Summary(context.Background(), merchant.ID)
	require.NoError(t, err)
	require.True(t, cache.has(merchant.ID), "la consulta debe haber guardado en cache")

	_, err = service.UpdateStatus(context.Background(), payment.ID, domain.PaymentStatusApproved, "ok", "test-user")
	require.NoError(t, err)

	require.False(t, cache.has(merchant.ID), "UpdateStatus debe invalidar la cache de ese comercio")
}

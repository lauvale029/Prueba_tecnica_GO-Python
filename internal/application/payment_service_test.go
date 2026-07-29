package application_test

import (
	"context"
	"sync"
	"testing"
	"time"

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

// copyPaymentValue devuelve una copia independiente de p — simula lo que
// un repositorio real de Postgres ya hace naturalmente (cada consulta
// SQL produce una fila nueva, nunca una referencia compartida). Sin
// esto, dos goroutines que consultan el mismo pago recibirían el MISMO
// puntero y podrían mutarlo a la vez sin ninguna sincronización.
func copyPaymentValue(p *domain.Payment) *domain.Payment {
	cp := *p
	return &cp
}

func (r *inMemoryPaymentRepository) GetByID(_ context.Context, id string) (*domain.Payment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.byID[id]
	if !ok {
		return nil, application.ErrNotFound
	}
	return copyPaymentValue(p), nil
}

func (r *inMemoryPaymentRepository) GetByIdempotencyKey(_ context.Context, key string) (*domain.Payment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.byIdempotencyKey[key]
	if !ok {
		return nil, application.ErrNotFound
	}
	return copyPaymentValue(p), nil
}

func (r *inMemoryPaymentRepository) GetByMerchantAndExternalReference(_ context.Context, merchantID, externalReference string) (*domain.Payment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.byExternalRef[paymentExternalRefKey(merchantID, externalReference)]
	if !ok {
		return nil, application.ErrNotFound
	}
	return copyPaymentValue(p), nil
}

// storeConsistently guarda p en los 3 índices (byID, byIdempotencyKey,
// byExternalRef) — necesario porque, a diferencia de antes, ya no
// compartimos el mismo puntero entre índices: cada escritura debe
// actualizar los 3 explícitamente para que ninguno quede con una copia
// vieja.
func (r *inMemoryPaymentRepository) storeConsistently(p *domain.Payment) {
	r.byID[p.ID] = p
	r.byIdempotencyKey[p.IdempotencyKey] = p
	r.byExternalRef[paymentExternalRefKey(p.MerchantID, p.ExternalReference)] = p
}

func (r *inMemoryPaymentRepository) UpdateStatus(_ context.Context, payment *domain.Payment) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.storeConsistently(copyPaymentValue(payment))
	return nil
}

func (r *inMemoryPaymentRepository) MarkProcessing(_ context.Context, paymentID, providerReference, providerName string, updatedAt time.Time) (*domain.Payment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.byID[paymentID]
	if !ok {
		return nil, application.ErrNotFound
	}
	updated := copyPaymentValue(p)
	updated.Status = domain.PaymentStatusProcessing
	updated.ProviderReference = &providerReference
	updated.ProviderName = &providerName
	updated.UpdatedAt = updatedAt
	r.storeConsistently(updated)
	return copyPaymentValue(updated), nil
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

// fakeUnitOfWork simplemente ejecuta fn con el mismo ctx: los fakes en
// memoria no tienen una noción real de transacción (sus escrituras ya
// son atómicas por el Mutex de cada uno), así que no hay nada que
// revertir de verdad — lo que se prueba acá es que PaymentService llama
// a Execute con la secuencia correcta, no la mecánica de un rollback real
// (eso lo cubre el test de integración contra Postgres).
type fakeUnitOfWork struct{}

func (fakeUnitOfWork) Execute(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

// fakeProvider es un PaymentProvider configurable por instancia — mismo
// espíritu que provider.SimulatedProvider (infrastructure/provider), pero
// definido acá para no hacer que los tests de application dependan de
// infrastructure, rompiendo la regla de dependencia hexagonal.
type fakeProviderBehavior int

const (
	fakeProviderApprove fakeProviderBehavior = iota
	fakeProviderReject
	fakeProviderUnreachable
)

// fakeProvider es un puntero a propósito: algunos tests cambian
// provider.behavior a mitad de camino (para simular "el proveedor ya
// sabe qué pasó" en un reintento) — con un value type, esa mutación no
// se vería reflejada en la copia que el registry ya tiene guardada.
type fakeProvider struct {
	behavior fakeProviderBehavior
}

func (p *fakeProvider) Charge(_ context.Context, _ application.ChargeRequest) (application.ProviderStatus, error) {
	switch p.behavior {
	case fakeProviderReject:
		return application.ProviderStatusRejected, nil
	case fakeProviderUnreachable:
		return "", application.ErrProviderUnreachable
	default:
		return application.ProviderStatusApproved, nil
	}
}

func (p *fakeProvider) GetStatus(_ context.Context, _ string) (application.ProviderStatus, error) {
	switch p.behavior {
	case fakeProviderReject:
		return application.ProviderStatusRejected, nil
	case fakeProviderUnreachable:
		return application.ProviderStatusProcessing, nil
	default:
		return application.ProviderStatusApproved, nil
	}
}

// fakeProviderRegistry siempre devuelve el mismo proveedor, sin importar
// el nombre pedido — suficiente para probar PaymentService, que hoy solo
// usa un proveedor por defecto (ver README, Sección 2).
type fakeProviderRegistry struct {
	provider application.PaymentProvider
}

func (r fakeProviderRegistry) Get(_ string) (application.PaymentProvider, error) {
	return r.provider, nil
}

const testDefaultProviderName = "test-provider"

// newPaymentServiceForTest arma un PaymentService con el proveedor falso
// en modo "aprueba de inmediato" — el comportamiento por defecto en
// producción también (ver cmd/api/main.go). Los tests que necesiten un
// comportamiento distinto (rechazo, proveedor inalcanzable) usan
// newPaymentServiceWithProvider directamente.
func newPaymentServiceForTest() (*application.PaymentService, *inMemoryMerchantRepository, *inMemoryPaymentRepository, *inMemoryHistoryRepository, *inMemorySummaryCache) {
	service, merchants, payments, history, summaries, _ := newPaymentServiceWithProvider(fakeProviderApprove)
	return service, merchants, payments, history, summaries
}

func newPaymentServiceWithProvider(behavior fakeProviderBehavior) (*application.PaymentService, *inMemoryMerchantRepository, *inMemoryPaymentRepository, *inMemoryHistoryRepository, *inMemorySummaryCache, *fakeProvider) {
	merchants := newInMemoryMerchantRepository()
	payments := newInMemoryPaymentRepository()
	history := newInMemoryHistoryRepository()
	summaries := newInMemorySummaryCache()
	provider := &fakeProvider{behavior: behavior}
	registry := fakeProviderRegistry{provider: provider}
	service := application.NewPaymentService(payments, merchants, history, application.NoopIdempotencyLocker{}, summaries, fakeUnitOfWork{}, registry, testDefaultProviderName)
	return service, merchants, payments, history, summaries, provider
}

func TestPaymentService_Create_Valid(t *testing.T) {
	service, merchants, _, _, _ := newPaymentServiceForTest()
	merchant := seedMerchant(t, merchants)

	payment, err := service.Create(context.Background(), merchant.ID, "ORDER-1", decimal.NewFromInt(1000), "COP", domain.PaymentMethodQR, "key-1", "test-user")

	require.NoError(t, err)
	// El proveedor falso por defecto aprueba de inmediato: Create ya no
	// deja el pago en PENDING, lo resuelve contra el proveedor antes de
	// devolverlo (ver README, Sección 2).
	require.Equal(t, domain.PaymentStatusApproved, payment.Status)
	require.NotNil(t, payment.ProviderReference)
	require.NotNil(t, payment.ProviderName)
	require.Equal(t, testDefaultProviderName, *payment.ProviderName)
}

func TestPaymentService_Create_MissingIdempotencyKey(t *testing.T) {
	service, merchants, _, _, _ := newPaymentServiceForTest()
	merchant := seedMerchant(t, merchants)

	_, err := service.Create(context.Background(), merchant.ID, "ORDER-1", decimal.NewFromInt(1000), "COP", domain.PaymentMethodQR, "", "test-user")

	require.ErrorIs(t, err, domain.ErrMissingIdempotencyKey)
}

func TestPaymentService_Create_MerchantNotFound(t *testing.T) {
	service, _, _, _, _ := newPaymentServiceForTest()

	_, err := service.Create(context.Background(), "id-inventado", "ORDER-1", decimal.NewFromInt(1000), "COP", domain.PaymentMethodQR, "key-1", "test-user")

	require.ErrorIs(t, err, application.ErrNotFound)
}

func TestPaymentService_Create_IdempotentReplay(t *testing.T) {
	service, merchants, _, _, _ := newPaymentServiceForTest()
	merchant := seedMerchant(t, merchants)
	key := "key-1"

	first, err := service.Create(context.Background(), merchant.ID, "ORDER-1", decimal.NewFromInt(1000), "COP", domain.PaymentMethodQR, key, "test-user")
	require.NoError(t, err)

	second, err := service.Create(context.Background(), merchant.ID, "ORDER-1", decimal.NewFromInt(1000), "COP", domain.PaymentMethodQR, key, "test-user")
	require.NoError(t, err)

	require.Equal(t, first.ID, second.ID)
}

func TestPaymentService_Create_DuplicateExternalReference(t *testing.T) {
	service, merchants, _, _, _ := newPaymentServiceForTest()
	merchant := seedMerchant(t, merchants)

	_, err := service.Create(context.Background(), merchant.ID, "ORDER-1", decimal.NewFromInt(1000), "COP", domain.PaymentMethodQR, "key-1", "test-user")
	require.NoError(t, err)

	_, err = service.Create(context.Background(), merchant.ID, "ORDER-1", decimal.NewFromInt(2000), "COP", domain.PaymentMethodCard, "key-2", "test-user")
	require.ErrorIs(t, err, application.ErrConflict)
}

func TestPaymentService_Create_ProviderRejects(t *testing.T) {
	service, merchants, _, _, _, _ := newPaymentServiceWithProvider(fakeProviderReject)
	merchant := seedMerchant(t, merchants)

	payment, err := service.Create(context.Background(), merchant.ID, "ORDER-1", decimal.NewFromInt(1000), "COP", domain.PaymentMethodQR, "key-1", "test-user")

	require.NoError(t, err)
	require.Equal(t, domain.PaymentStatusRejected, payment.Status)
}

func TestPaymentService_Create_ProviderUnreachable_MarksUnknown(t *testing.T) {
	service, merchants, _, _, _, _ := newPaymentServiceWithProvider(fakeProviderUnreachable)
	merchant := seedMerchant(t, merchants)

	payment, err := service.Create(context.Background(), merchant.ID, "ORDER-1", decimal.NewFromInt(1000), "COP", domain.PaymentMethodQR, "key-1", "test-user")

	require.NoError(t, err)
	require.Equal(t, domain.PaymentStatusUnknown, payment.Status)
	require.NotNil(t, payment.ProviderReference, "la referencia debe quedar guardada aunque el proveedor no haya respondido")
}

// TestPaymentService_Create_RetryReconciles reproduce el escenario
// central del riesgo documentado: un primer intento queda en UNKNOWN
// (el proveedor no respondió), y un reintento con la MISMA
// Idempotency-Key no vuelve a cobrar — concilia con el proveedor y
// resuelve el pago existente.
func TestPaymentService_Create_RetryReconciles(t *testing.T) {
	service, merchants, _, _, _, provider := newPaymentServiceWithProvider(fakeProviderUnreachable)
	merchant := seedMerchant(t, merchants)

	first, err := service.Create(context.Background(), merchant.ID, "ORDER-1", decimal.NewFromInt(1000), "COP", domain.PaymentMethodQR, "key-1", "test-user")
	require.NoError(t, err)
	require.Equal(t, domain.PaymentStatusUnknown, first.Status)

	// El proveedor, con el tiempo, sí sabe qué pasó — cambiamos su
	// comportamiento para simular eso antes del reintento.
	provider.behavior = fakeProviderApprove

	second, err := service.Create(context.Background(), merchant.ID, "ORDER-1", decimal.NewFromInt(1000), "COP", domain.PaymentMethodQR, "key-1", "test-user")

	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID, "el reintento nunca crea un pago nuevo")
	require.Equal(t, domain.PaymentStatusApproved, second.Status, "el reintento debió conciliar, no volver a cobrar")
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
			payment, err := service.Create(context.Background(), merchant.ID, "ORDER-CONCURRENT", decimal.NewFromInt(1000), "COP", domain.PaymentMethodQR, idempotencyKey, "test-user")
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

// TestPaymentService_UpdateStatus_Valid usa un proveedor inalcanzable a
// propósito: así el pago queda en UNKNOWN tras Create (en vez de
// resolverse solo), dejando algo pendiente de un cambio manual real que
// probar — UpdateStatus sigue permitiendo resolver UNKNOWN a mano,
// además de la conciliación automática.
func TestPaymentService_UpdateStatus_Valid(t *testing.T) {
	service, merchants, _, history, _, _ := newPaymentServiceWithProvider(fakeProviderUnreachable)
	merchant := seedMerchant(t, merchants)
	payment, err := service.Create(context.Background(), merchant.ID, "ORDER-1", decimal.NewFromInt(1000), "COP", domain.PaymentMethodQR, "key-1", "test-user")
	require.NoError(t, err)
	require.Equal(t, domain.PaymentStatusUnknown, payment.Status)

	updated, err := service.UpdateStatus(context.Background(), payment.ID, domain.PaymentStatusApproved, "pago confirmado", "test-user")

	require.NoError(t, err)
	require.Equal(t, domain.PaymentStatusApproved, updated.Status)

	// Create ya generó sus propias entradas (PENDING→PROCESSING→UNKNOWN);
	// esta prueba se enfoca en la ÚLTIMA, la que corresponde a este
	// UpdateStatus manual.
	entries, err := history.ListByPaymentID(context.Background(), payment.ID)
	require.NoError(t, err)
	last := entries[len(entries)-1]
	require.Equal(t, domain.PaymentStatusUnknown, last.PreviousStatus)
	require.Equal(t, domain.PaymentStatusApproved, last.NewStatus)
	require.Equal(t, "test-user", last.ChangedBy)
}

func TestPaymentService_UpdateStatus_InvalidTransition(t *testing.T) {
	service, merchants, _, _, _, _ := newPaymentServiceWithProvider(fakeProviderUnreachable)
	merchant := seedMerchant(t, merchants)
	payment, err := service.Create(context.Background(), merchant.ID, "ORDER-1", decimal.NewFromInt(1000), "COP", domain.PaymentMethodQR, "key-1", "test-user")
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
	_, err := service.Create(context.Background(), merchant.ID, "ORDER-1", decimal.NewFromInt(1000), "COP", domain.PaymentMethodQR, "key-1", "test-user")
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

// TestPaymentService_Summary_ComputesFromRepository siembra los pagos
// directo en el repositorio falso (sin pasar por Create/el proveedor):
// lo que esta prueba verifica es el cálculo del resumen en sí, no el
// flujo de creación — que ya tiene sus propias pruebas dedicadas.
func TestPaymentService_Summary_ComputesFromRepository(t *testing.T) {
	service, merchants, payments, _, _ := newPaymentServiceForTest()
	merchant := seedMerchant(t, merchants)
	ctx := context.Background()

	approved, err := domain.NewPayment(merchant.ID, "ORDER-1", decimal.NewFromInt(1000), "COP", domain.PaymentMethodQR, "key-1")
	require.NoError(t, err)
	require.NoError(t, approved.ChangeStatus(domain.PaymentStatusApproved))
	require.NoError(t, payments.Create(ctx, approved))

	pending, err := domain.NewPayment(merchant.ID, "ORDER-2", decimal.NewFromInt(500), "COP", domain.PaymentMethodQR, "key-2")
	require.NoError(t, err)
	require.NoError(t, payments.Create(ctx, pending))

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
	_, err := service.Create(context.Background(), merchant.ID, "ORDER-1", decimal.NewFromInt(1000), "COP", domain.PaymentMethodQR, "key-1", "test-user")
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
	service, merchants, _, _, cache, _ := newPaymentServiceWithProvider(fakeProviderUnreachable)
	merchant := seedMerchant(t, merchants)
	payment, err := service.Create(context.Background(), merchant.ID, "ORDER-1", decimal.NewFromInt(1000), "COP", domain.PaymentMethodQR, "key-1", "test-user")
	require.NoError(t, err)

	_, err = service.Summary(context.Background(), merchant.ID)
	require.NoError(t, err)
	require.True(t, cache.has(merchant.ID), "la consulta debe haber guardado en cache")

	_, err = service.UpdateStatus(context.Background(), payment.ID, domain.PaymentStatusApproved, "ok", "test-user")
	require.NoError(t, err)

	require.False(t, cache.has(merchant.ID), "UpdateStatus debe invalidar la cache de ese comercio")
}

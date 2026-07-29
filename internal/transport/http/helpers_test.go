package http_test

import (
	"context"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/shopspring/decimal"

	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/application"
	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/domain"
	authinfra "github.com/lauvale029/Prueba_tecnica_GO-Python/internal/infrastructure/auth"
	transporthttp "github.com/lauvale029/Prueba_tecnica_GO-Python/internal/transport/http"
)

// testJWTSecret firma los tokens usados en este paquete de tests; no
// tiene relación con el JWT_SECRET real de .env.
const testJWTSecret = "test-secret-no-usar-en-produccion"

// fakeMerchantRepository y fakePaymentRepository reutilizan
// application.ErrNotFound/ErrConflict a propósito: los handlers
// (código de producción) ya los conocen, así que los tests ejercitan la
// traducción real a códigos HTTP.
type fakeMerchantRepository struct {
	mu              sync.Mutex
	merchants       map[string]*domain.Merchant
	documentNumbers map[string]bool
}

func newFakeMerchantRepository() *fakeMerchantRepository {
	return &fakeMerchantRepository{
		merchants:       make(map[string]*domain.Merchant),
		documentNumbers: make(map[string]bool),
	}
}

func (f *fakeMerchantRepository) Create(_ context.Context, merchant *domain.Merchant) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.documentNumbers[merchant.DocumentNumber] {
		return application.ErrConflict
	}
	f.merchants[merchant.ID] = merchant
	f.documentNumbers[merchant.DocumentNumber] = true
	return nil
}

func (f *fakeMerchantRepository) GetByID(_ context.Context, id string) (*domain.Merchant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	merchant, ok := f.merchants[id]
	if !ok {
		return nil, application.ErrNotFound
	}
	return merchant, nil
}

type fakePaymentRepository struct {
	mu               sync.Mutex
	byID             map[string]*domain.Payment
	byIdempotencyKey map[string]*domain.Payment
	byExternalRefKey map[string]*domain.Payment
}

func newFakePaymentRepository() *fakePaymentRepository {
	return &fakePaymentRepository{
		byID:             make(map[string]*domain.Payment),
		byIdempotencyKey: make(map[string]*domain.Payment),
		byExternalRefKey: make(map[string]*domain.Payment),
	}
}

func externalRefKey(merchantID, externalReference string) string {
	return merchantID + "|" + externalReference
}

func (f *fakePaymentRepository) Create(_ context.Context, payment *domain.Payment) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if _, exists := f.byIdempotencyKey[payment.IdempotencyKey]; exists {
		return application.ErrConflict
	}
	refKey := externalRefKey(payment.MerchantID, payment.ExternalReference)
	if _, exists := f.byExternalRefKey[refKey]; exists {
		return application.ErrConflict
	}

	f.byID[payment.ID] = payment
	f.byIdempotencyKey[payment.IdempotencyKey] = payment
	f.byExternalRefKey[refKey] = payment
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

func (f *fakePaymentRepository) GetByID(_ context.Context, id string) (*domain.Payment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	payment, ok := f.byID[id]
	if !ok {
		return nil, application.ErrNotFound
	}
	return copyPaymentValue(payment), nil
}

func (f *fakePaymentRepository) GetByIdempotencyKey(_ context.Context, idempotencyKey string) (*domain.Payment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	payment, ok := f.byIdempotencyKey[idempotencyKey]
	if !ok {
		return nil, application.ErrNotFound
	}
	return copyPaymentValue(payment), nil
}

func (f *fakePaymentRepository) GetByMerchantAndExternalReference(_ context.Context, merchantID, externalReference string) (*domain.Payment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	payment, ok := f.byExternalRefKey[externalRefKey(merchantID, externalReference)]
	if !ok {
		return nil, application.ErrNotFound
	}
	return copyPaymentValue(payment), nil
}

// storeConsistently guarda payment en los 3 índices — necesario porque
// ya no compartimos el mismo puntero entre índices (ver
// copyPaymentValue): cada escritura debe actualizar los 3 explícitamente.
func (f *fakePaymentRepository) storeConsistently(payment *domain.Payment) {
	f.byID[payment.ID] = payment
	f.byIdempotencyKey[payment.IdempotencyKey] = payment
	f.byExternalRefKey[externalRefKey(payment.MerchantID, payment.ExternalReference)] = payment
}

func (f *fakePaymentRepository) UpdateStatus(_ context.Context, payment *domain.Payment) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.storeConsistently(copyPaymentValue(payment))
	return nil
}

func (f *fakePaymentRepository) MarkProcessing(_ context.Context, paymentID, providerReference, providerName string, updatedAt time.Time) (*domain.Payment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	payment, ok := f.byID[paymentID]
	if !ok {
		return nil, application.ErrNotFound
	}
	updated := copyPaymentValue(payment)
	updated.Status = domain.PaymentStatusProcessing
	updated.ProviderReference = &providerReference
	updated.ProviderName = &providerName
	updated.UpdatedAt = updatedAt
	f.storeConsistently(updated)
	return copyPaymentValue(updated), nil
}

// List/Count sí filtran de verdad (a diferencia del fake equivalente en
// internal/application): acá queremos probar que los query params del
// endpoint HTTP realmente se traducen en el filtrado correcto de punta a
// punta.
func (f *fakePaymentRepository) List(_ context.Context, filter application.PaymentFilter) ([]*domain.Payment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var result []*domain.Payment
	for _, p := range f.byID {
		if matchesPaymentFilter(p, filter) {
			result = append(result, p)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })

	start := (filter.Page - 1) * filter.Limit
	if start > len(result) {
		start = len(result)
	}
	end := start + filter.Limit
	if end > len(result) {
		end = len(result)
	}
	return result[start:end], nil
}

func (f *fakePaymentRepository) Count(_ context.Context, filter application.PaymentFilter) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var count int64
	for _, p := range f.byID {
		if matchesPaymentFilter(p, filter) {
			count++
		}
	}
	return count, nil
}

// GetSummaryByMerchantID sí calcula de verdad (a diferencia del fake de
// internal/application, que solo necesita probar la lógica de
// PaymentService): acá queremos probar que el endpoint HTTP devuelva
// números reales.
func (f *fakePaymentRepository) GetSummaryByMerchantID(_ context.Context, merchantID string) (application.MerchantSummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	summary := application.MerchantSummary{MerchantID: merchantID, ApprovedAmount: decimal.Zero}
	for _, p := range f.byID {
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

func matchesPaymentFilter(p *domain.Payment, filter application.PaymentFilter) bool {
	if filter.MerchantID != nil && p.MerchantID != *filter.MerchantID {
		return false
	}
	if filter.Status != nil && p.Status != *filter.Status {
		return false
	}
	if filter.PaymentMethod != nil && p.PaymentMethod != *filter.PaymentMethod {
		return false
	}
	if filter.DateFrom != nil && p.CreatedAt.Before(*filter.DateFrom) {
		return false
	}
	if filter.DateTo != nil && p.CreatedAt.After(*filter.DateTo) {
		return false
	}
	return true
}

type fakeHistoryRepository struct {
	mu      sync.Mutex
	entries map[string][]*domain.PaymentStatusHistory
}

func newFakeHistoryRepository() *fakeHistoryRepository {
	return &fakeHistoryRepository{entries: make(map[string][]*domain.PaymentStatusHistory)}
}

func (f *fakeHistoryRepository) Create(_ context.Context, history *domain.PaymentStatusHistory) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries[history.PaymentID] = append(f.entries[history.PaymentID], history)
	return nil
}

func (f *fakeHistoryRepository) ListByPaymentID(_ context.Context, paymentID string) ([]*domain.PaymentStatusHistory, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.entries[paymentID], nil
}

// testApp agrupa lo que necesitan los tests de este paquete: la app de
// Fiber ya armada, los repositorios falsos para poder "sembrar" datos
// antes de hacer una petición HTTP, y un token válido para autenticar
// las peticiones a rutas protegidas.
type testApp struct {
	app       *fiber.App
	merchants *fakeMerchantRepository
	payments  *fakePaymentRepository
	history   *fakeHistoryRepository
	token     string
	provider  *fakeProvider
}

// test envía req a través de la app añadiendo antes el header
// Authorization con un token válido, para no repetirlo en cada test.
func (ta testApp) test(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+ta.token)
	return ta.app.Test(req)
}

// fakeUnitOfWork simplemente ejecuta fn con el mismo ctx — ver la nota
// equivalente en internal/application/payment_service_test.go.
type fakeUnitOfWork struct{}

func (fakeUnitOfWork) Execute(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

// fakeProvider es configurable por instancia — el comportamiento por
// defecto (setupApp) aprueba de inmediato; setupAppWithProviderBehavior
// permite a un test puntual usar uno que no resuelve nada (para poder
// seguir probando la transición manual PENDING/UNKNOWN→APPROVED vía
// PATCH /status, sin que el proveedor se adelante). Este paquete prueba
// el cableado HTTP, no las reglas de negocio del proveedor en sí (esas
// tienen su propia cobertura en internal/application y
// internal/infrastructure/provider).
type fakeProviderBehavior int

const (
	fakeProviderApprove fakeProviderBehavior = iota
	fakeProviderUnreachable
)

// fakeProvider es un puntero a propósito: algunos tests cambian
// provider.behavior a mitad de camino (para simular "el proveedor ya
// sabe qué pasó" antes de conciliar) — con un value type, esa mutación
// no se vería reflejada en la copia que el registry ya tiene guardada.
type fakeProvider struct {
	behavior fakeProviderBehavior
}

func (p *fakeProvider) Charge(_ context.Context, _ application.ChargeRequest) (application.ProviderStatus, error) {
	if p.behavior == fakeProviderUnreachable {
		return "", application.ErrProviderUnreachable
	}
	return application.ProviderStatusApproved, nil
}

func (p *fakeProvider) GetStatus(_ context.Context, _ string) (application.ProviderStatus, error) {
	if p.behavior == fakeProviderUnreachable {
		return application.ProviderStatusProcessing, nil
	}
	return application.ProviderStatusApproved, nil
}

type fakeProviderRegistry struct {
	provider *fakeProvider
}

func (r fakeProviderRegistry) Get(_ string) (application.PaymentProvider, error) {
	return r.provider, nil
}

const testDefaultProviderName = "test-provider"

func setupApp() testApp {
	return setupAppWithProviderBehavior(fakeProviderApprove)
}

func setupAppWithProviderBehavior(behavior fakeProviderBehavior) testApp {
	merchantRepo := newFakeMerchantRepository()
	paymentRepo := newFakePaymentRepository()
	historyRepo := newFakeHistoryRepository()

	provider := &fakeProvider{behavior: behavior}
	registry := fakeProviderRegistry{provider: provider}
	paymentService := application.NewPaymentService(paymentRepo, merchantRepo, historyRepo, application.NoopIdempotencyLocker{}, application.NoopSummaryCache{}, fakeUnitOfWork{}, registry, testDefaultProviderName)
	paymentHandler := transporthttp.NewPaymentHandler(paymentService)

	merchantService := application.NewMerchantService(merchantRepo)
	merchantHandler := transporthttp.NewMerchantHandler(merchantService, paymentService)

	authHandler := transporthttp.NewAuthHandler("mova-service", "Mova-Service#123", testJWTSecret, time.Hour)

	token, _, err := authinfra.IssueToken(testJWTSecret, "mova-service", time.Hour)
	if err != nil {
		panic(err)
	}

	return testApp{
		app:       transporthttp.NewRouter(merchantHandler, paymentHandler, authHandler, testJWTSecret),
		merchants: merchantRepo,
		payments:  paymentRepo,
		history:   historyRepo,
		token:     token,
		provider:  provider,
	}
}
package http_test

import (
	"context"
	"sync"

	"github.com/gofiber/fiber/v2"

	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/application"
	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/domain"
	transporthttp "github.com/lauvale029/Prueba_tecnica_GO-Python/internal/transport/http"
)

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
	mu                sync.Mutex
	byID              map[string]*domain.Payment
	byIdempotencyKey  map[string]*domain.Payment
	byExternalRefKey  map[string]*domain.Payment
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

func (f *fakePaymentRepository) GetByID(_ context.Context, id string) (*domain.Payment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	payment, ok := f.byID[id]
	if !ok {
		return nil, application.ErrNotFound
	}
	return payment, nil
}

func (f *fakePaymentRepository) GetByIdempotencyKey(_ context.Context, idempotencyKey string) (*domain.Payment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	payment, ok := f.byIdempotencyKey[idempotencyKey]
	if !ok {
		return nil, application.ErrNotFound
	}
	return payment, nil
}

func (f *fakePaymentRepository) GetByMerchantAndExternalReference(_ context.Context, merchantID, externalReference string) (*domain.Payment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	payment, ok := f.byExternalRefKey[externalRefKey(merchantID, externalReference)]
	if !ok {
		return nil, application.ErrNotFound
	}
	return payment, nil
}

func (f *fakePaymentRepository) UpdateStatus(_ context.Context, payment *domain.Payment) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.byID[payment.ID] = payment
	return nil
}

// testApp agrupa lo que necesitan los tests de este paquete: la app de
// Fiber ya armada, y los dos repositorios falsos para poder "sembrar"
// datos antes de hacer una petición HTTP.
type testApp struct {
	app       *fiber.App
	merchants *fakeMerchantRepository
	payments  *fakePaymentRepository
}

func setupApp() testApp {
	merchantRepo := newFakeMerchantRepository()
	paymentRepo := newFakePaymentRepository()

	merchantService := application.NewMerchantService(merchantRepo)
	paymentService := application.NewPaymentService(paymentRepo, merchantRepo, application.NoopIdempotencyLocker{})

	merchantHandler := transporthttp.NewMerchantHandler(merchantService)
	paymentHandler := transporthttp.NewPaymentHandler(paymentService)

	return testApp{
		app:       transporthttp.NewRouter(merchantHandler, paymentHandler),
		merchants: merchantRepo,
		payments:  paymentRepo,
	}
}
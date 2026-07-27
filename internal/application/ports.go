package application

import (
	"context"
	"time"

	"github.com/shopspring/decimal"

	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/domain"
)

// MerchantSummary es el resultado agregado de GET /merchants/{id}/summary.
type MerchantSummary struct {
	MerchantID       string
	TotalPayments    int64
	ApprovedPayments int64
	RejectedPayments int64
	PendingPayments  int64
	ApprovedAmount   decimal.Decimal
}

// PaymentFilter agrupa los filtros opcionales para listar pagos. Un
// campo en nil significa "sin filtro" para ese campo
type PaymentFilter struct {
	MerchantID    *string
	Status        *domain.PaymentStatus
	PaymentMethod *domain.PaymentMethod
	DateFrom      *time.Time
	DateTo        *time.Time
	Page          int
	Limit         int
}

// MerchantRepository es el puerto que la capa de aplicación espera de
// quien persista comercios
type MerchantRepository interface {
	Create(ctx context.Context, merchant *domain.Merchant) error
	GetByID(ctx context.Context, id string) (*domain.Merchant, error)
}

// PaymentRepository es el puerto para persistir y consultar pagos.
type PaymentRepository interface {
	Create(ctx context.Context, payment *domain.Payment) error
	GetByID(ctx context.Context, id string) (*domain.Payment, error)
	GetByIdempotencyKey(ctx context.Context, idempotencyKey string) (*domain.Payment, error)
	GetByMerchantAndExternalReference(ctx context.Context, merchantID, externalReference string) (*domain.Payment, error)
	UpdateStatus(ctx context.Context, payment *domain.Payment) error
	List(ctx context.Context, filter PaymentFilter) ([]*domain.Payment, error)
	Count(ctx context.Context, filter PaymentFilter) (int64, error)
	GetSummaryByMerchantID(ctx context.Context, merchantID string) (MerchantSummary, error)
}

// PaymentStatusHistoryRepository es el puerto para el historial de cambios
// de estado de un pago.
type PaymentStatusHistoryRepository interface {
	Create(ctx context.Context, history *domain.PaymentStatusHistory) error
	ListByPaymentID(ctx context.Context, paymentID string) ([]*domain.PaymentStatusHistory, error)
}

// IdempotencyLocker es un lock opcional y de mejor esfuerzo para reducir
// la contención en Postgres cuando llegan solicitudes concurrentes con la
// misma Idempotency-Key. No es necesario para la corrección: la
// restricción única de la base de datos es la garantía real, esto solo
// evita golpearla más veces de las necesarias bajo alta concurrencia.
type IdempotencyLocker interface {
	// Acquire intenta tomar el lock para key. release nunca es nil —
	// siempre es seguro llamarlo, incluso si acquired es false.
	Acquire(ctx context.Context, key string) (release func(), acquired bool)
}

// NoopIdempotencyLocker es la implementación usada cuando Redis no está
// configurado o no está disponible: nunca "consigue" el lock, así que el
// caso de uso siempre recae en la restricción única de Postgres. El
// sistema sigue siendo correcto, solo un poco menos eficiente bajo
// concurrencia extrema.
type NoopIdempotencyLocker struct{}

func (NoopIdempotencyLocker) Acquire(_ context.Context, _ string) (func(), bool) {
	return func() {}, false
}

// SummaryCache es una cache opcional y de mejor esfuerzo para
// GET /merchants/{id}/summary. No es necesaria para la corrección: si no
// hay nada en cache (o Redis no está disponible), el resumen se calcula
// directo contra Postgres.
type SummaryCache interface {
	Get(ctx context.Context, merchantID string) (MerchantSummary, bool)
	Set(ctx context.Context, merchantID string, summary MerchantSummary)
	Invalidate(ctx context.Context, merchantID string)
}

// NoopSummaryCache es la implementación usada cuando Redis no está
// configurado o no está disponible: siempre "falla" la búsqueda, así que
// el resumen se recalcula contra Postgres en cada petición. Correcto,
// solo un poco más lento.
type NoopSummaryCache struct{}

func (NoopSummaryCache) Get(_ context.Context, _ string) (MerchantSummary, bool) {
	return MerchantSummary{}, false
}
func (NoopSummaryCache) Set(_ context.Context, _ string, _ MerchantSummary) {}
func (NoopSummaryCache) Invalidate(_ context.Context, _ string)             {}

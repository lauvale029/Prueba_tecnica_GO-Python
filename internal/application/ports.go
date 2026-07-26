package application

import (
	"context"

	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/domain"
)

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

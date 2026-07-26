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

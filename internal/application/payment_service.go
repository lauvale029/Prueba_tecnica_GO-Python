package application

import (
	"context"
	"errors"
	"time"

	"github.com/shopspring/decimal"

	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/domain"
)

// idempotencyRetryDelay es cuánto esperamos, una sola vez, antes de
// revisar de nuevo si otra petición concurrente con la misma
// Idempotency-Key ya terminó de crear el pago.
const idempotencyRetryDelay = 50 * time.Millisecond

// PaymentService orquesta el caso de uso de pagos: valida con el
// dominio, verifica que el comercio exista, y aplica la estrategia de
// idempotencia/concurrencia (ver README, sección "Idempotencia y
// concurrencia").
type PaymentService struct {
	payments  PaymentRepository
	merchants MerchantRepository
	locker    IdempotencyLocker
}

func NewPaymentService(payments PaymentRepository, merchants MerchantRepository, locker IdempotencyLocker) *PaymentService {
	return &PaymentService{payments: payments, merchants: merchants, locker: locker}
}

// Create crea un pago nuevo, o devuelve el pago ya existente si
// idempotencyKey ya fue usada antes (idempotencia) o si otra petición
// concurrente con la misma key ganó la carrera (concurrencia).
func (s *PaymentService) Create(
	ctx context.Context,
	merchantID, externalReference string,
	amount decimal.Decimal,
	currency string,
	method domain.PaymentMethod,
	idempotencyKey string,
) (*domain.Payment, error) {
	if idempotencyKey == "" {
		return nil, domain.ErrMissingIdempotencyKey
	}

	// 1. Replay: si esta key ya se usó, devolvemos el pago original.
	if existing, err := s.payments.GetByIdempotencyKey(ctx, idempotencyKey); err == nil {
		return existing, nil
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	// 2. El comercio referenciado debe existir.
	if _, err := s.merchants.GetByID(ctx, merchantID); err != nil {
		return nil, err
	}

	// 3. Lock opcional en Redis (best-effort). Si no lo conseguimos,
	// esperamos un instante y volvemos a chequear una vez antes de
	// seguir de todas formas: la restricción única de Postgres decide al
	// final, pase lo que pase acá.
	release, acquired := s.locker.Acquire(ctx, idempotencyKey)
	defer release()

	if !acquired {
		time.Sleep(idempotencyRetryDelay)
		if existing, err := s.payments.GetByIdempotencyKey(ctx, idempotencyKey); err == nil {
			return existing, nil
		} else if !errors.Is(err, ErrNotFound) {
			return nil, err
		}
	}

	// 4. Validar con el dominio y persistir.
	payment, err := domain.NewPayment(merchantID, externalReference, amount, currency, method, idempotencyKey)
	if err != nil {
		return nil, err
	}

	if err := s.payments.Create(ctx, payment); err != nil {
		if !errors.Is(err, ErrConflict) {
			return nil, err
		}

		// 5. Postgres rechazó por una restricción única. Si es la misma
		// idempotencyKey, alguien más ganó la carrera: devolvemos su
		// pago (esto SÍ es un replay exitoso). Si no aparece nada con
		// esta key, el conflicto fue por external_reference duplicada:
		// un error real, no un replay.
		existing, getErr := s.payments.GetByIdempotencyKey(ctx, idempotencyKey)
		if getErr != nil {
			if errors.Is(getErr, ErrNotFound) {
				return nil, ErrConflict
			}
			return nil, getErr
		}
		return existing, nil
	}

	return payment, nil
}
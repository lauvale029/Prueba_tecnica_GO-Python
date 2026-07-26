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

// Valores de paginación por defecto y máximo. Exportados porque
// transport/http los usa para normalizar los query params antes de
// construir el PaymentFilter.
const (
	DefaultPage  = 1
	DefaultLimit = 100
	MaxLimit     = 100
)

// PaymentService orquesta el caso de uso de pagos: valida con el
// dominio, verifica que el comercio exista, y aplica la estrategia de
// idempotencia/concurrencia (ver README, sección "Idempotencia y
// concurrencia").
type PaymentService struct {
	payments  PaymentRepository
	merchants MerchantRepository
	history   PaymentStatusHistoryRepository
	locker    IdempotencyLocker
}

func NewPaymentService(payments PaymentRepository, merchants MerchantRepository, history PaymentStatusHistoryRepository, locker IdempotencyLocker) *PaymentService {
	return &PaymentService{payments: payments, merchants: merchants, history: history, locker: locker}
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

// Get consulta un pago existente por su ID.
func (s *PaymentService) Get(ctx context.Context, id string) (*domain.Payment, error) {
	return s.payments.GetByID(ctx, id)
}

// List devuelve los pagos que cumplen filter, junto con el total de
// coincidencias (sin paginar), para que el llamador pueda calcular
// cuántas páginas hay. Normaliza Page/Limit a valores seguros antes de
// consultar: sin esto, "sin parámetros" terminaría trayendo toda la
// tabla de una sola vez.
func (s *PaymentService) List(ctx context.Context, filter PaymentFilter) ([]*domain.Payment, int64, error) {
	if filter.Page < 1 {
		filter.Page = DefaultPage
	}
	if filter.Limit <= 0 {
		filter.Limit = DefaultLimit
	}
	if filter.Limit > MaxLimit {
		filter.Limit = MaxLimit
	}

	payments, err := s.payments.List(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	total, err := s.payments.Count(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	return payments, total, nil
}

// UpdateStatus aplica una transición de estado (domain.Payment.ChangeStatus
// decide si es válida) y deja el registro correspondiente en el
// historial.
func (s *PaymentService) UpdateStatus(ctx context.Context, paymentID string, newStatus domain.PaymentStatus, reason, changedBy string) (*domain.Payment, error) {
	payment, err := s.payments.GetByID(ctx, paymentID)
	if err != nil {
		return nil, err
	}

	previousStatus := payment.Status
	if err := payment.ChangeStatus(newStatus); err != nil {
		return nil, err
	}

	if err := s.payments.UpdateStatus(ctx, payment); err != nil {
		return nil, err
	}

	history := domain.NewPaymentStatusHistory(payment.ID, previousStatus, payment.Status, reason, changedBy)
	if err := s.history.Create(ctx, history); err != nil {
		return nil, err
	}

	return payment, nil
}

// History devuelve el historial de cambios de estado de un pago,
// verificando primero que el pago exista (para poder responder 404 en
// vez de una lista vacía silenciosa si el ID no existe).
func (s *PaymentService) History(ctx context.Context, paymentID string) ([]*domain.PaymentStatusHistory, error) {
	if _, err := s.payments.GetByID(ctx, paymentID); err != nil {
		return nil, err
	}
	return s.history.ListByPaymentID(ctx, paymentID)
}
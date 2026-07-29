package application

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
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

// Razones fijas para las entradas de historial que genera el propio
// sistema (no un cambio manual vía PATCH /status).
const (
	reasonSentToProvider   = "enviado al proveedor de pagos"
	reasonProviderNoAnswer = "el proveedor no respondió"
	reasonReconciled       = "conciliado con el proveedor"
)

// PaymentService orquesta el caso de uso de pagos: valida con el
// dominio, verifica que el comercio exista, aplica la estrategia de
// idempotencia/concurrencia, y — desde Sección 2 — envía el cobro a un
// proveedor de pagos externo y concilia operaciones inciertas.
type PaymentService struct {
	payments            PaymentRepository
	merchants           MerchantRepository
	history             PaymentStatusHistoryRepository
	locker              IdempotencyLocker
	summaries           SummaryCache
	uow                 UnitOfWork
	providers           ProviderRegistry
	defaultProviderName string
}

func NewPaymentService(
	payments PaymentRepository,
	merchants MerchantRepository,
	history PaymentStatusHistoryRepository,
	locker IdempotencyLocker,
	summaries SummaryCache,
	uow UnitOfWork,
	providers ProviderRegistry,
	defaultProviderName string,
) *PaymentService {
	return &PaymentService{
		payments:            payments,
		merchants:           merchants,
		history:             history,
		locker:              locker,
		summaries:           summaries,
		uow:                 uow,
		providers:           providers,
		defaultProviderName: defaultProviderName,
	}
}

// Create crea un pago nuevo y lo envía al proveedor de pagos configurado,
// o resuelve el pago ya existente si idempotencyKey ya fue usada antes
// (idempotencia) o si otra petición concurrente con la misma key ganó la
// carrera (concurrencia). changedBy identifica quién origina la
// operación (el subject del JWT autenticado), para el historial.
func (s *PaymentService) Create(
	ctx context.Context,
	merchantID, externalReference string,
	amount decimal.Decimal,
	currency string,
	method domain.PaymentMethod,
	idempotencyKey string,
	changedBy string,
) (*domain.Payment, error) {
	if idempotencyKey == "" {
		return nil, domain.ErrMissingIdempotencyKey
	}

	// 1. Replay: si esta key ya se usó, resolvemos su estado incierto (si
	// aplica — ver resolveIfUncertain) y devolvemos el pago. Un reintento
	// NUNCA vuelve a llamar al proveedor directamente.
	if existing, err := s.payments.GetByIdempotencyKey(ctx, idempotencyKey); err == nil {
		return s.resolveIfUncertain(ctx, existing, changedBy)
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
			return s.resolveIfUncertain(ctx, existing, changedBy)
		} else if !errors.Is(err, ErrNotFound) {
			return nil, err
		}
	}

	// 4. Validar con el dominio y persistir en PENDING.
	payment, err := domain.NewPayment(merchantID, externalReference, amount, currency, method, idempotencyKey)
	if err != nil {
		return nil, err
	}

	if err := s.payments.Create(ctx, payment); err != nil {
		if !errors.Is(err, ErrConflict) {
			return nil, err
		}

		// 5. Postgres rechazó por una restricción única. Si es la misma
		// idempotencyKey, alguien más ganó la carrera: resolvemos SU pago
		// (esto sí es un replay exitoso). Si no aparece nada con esta
		// key, el conflicto fue por external_reference duplicada: un
		// error real, no un replay.
		existing, getErr := s.payments.GetByIdempotencyKey(ctx, idempotencyKey)
		if getErr != nil {
			if errors.Is(getErr, ErrNotFound) {
				return nil, ErrConflict
			}
			return nil, getErr
		}
		return s.resolveIfUncertain(ctx, existing, changedBy)
	}

	// 6. Pago nuevo, sin ningún intento previo: enviarlo al proveedor.
	return s.chargeWithProvider(ctx, payment, changedBy)
}

// resolveIfUncertain concilia con el proveedor si existing quedó en un
// estado incierto de un intento anterior (PROCESSING/UNKNOWN). Si ya
// está resuelto (APPROVED/REJECTED/CANCELLED) o ni siquiera se envió al
// proveedor todavía (PENDING), se devuelve tal cual, sin tocar nada.
func (s *PaymentService) resolveIfUncertain(ctx context.Context, existing *domain.Payment, changedBy string) (*domain.Payment, error) {
	if existing.Status != domain.PaymentStatusProcessing && existing.Status != domain.PaymentStatusUnknown {
		return existing, nil
	}
	return s.reconcileWithProvider(ctx, existing, changedBy)
}

// chargeWithProvider marca payment como PROCESSING (guardando una
// referencia de operación nueva ANTES de llamar al proveedor, de forma
// atómica con su entrada de historial — ver README, Sección 2), lo envía
// al proveedor configurado, y resuelve el resultado.
func (s *PaymentService) chargeWithProvider(ctx context.Context, payment *domain.Payment, changedBy string) (*domain.Payment, error) {
	providerReference := uuid.New().String()
	previousStatus := payment.Status

	err := s.uow.Execute(ctx, func(txCtx context.Context) error {
		updated, err := s.payments.MarkProcessing(txCtx, payment.ID, providerReference, s.defaultProviderName, time.Now().UTC())
		if err != nil {
			return err
		}
		*payment = *updated

		history := domain.NewPaymentStatusHistory(payment.ID, previousStatus, payment.Status, reasonSentToProvider, changedBy)
		return s.history.Create(txCtx, history)
	})
	if err != nil {
		return nil, err
	}

	provider, err := s.providers.Get(s.defaultProviderName)
	if err != nil {
		return nil, err
	}

	status, chargeErr := provider.Charge(ctx, ChargeRequest{
		ProviderReference: providerReference,
		Amount:            payment.Amount.Amount,
		Currency:          payment.Amount.Currency,
		PaymentMethod:     payment.PaymentMethod,
	})
	if chargeErr != nil {
		if !errors.Is(chargeErr, ErrProviderUnreachable) {
			return nil, chargeErr
		}
		return s.applyStatusChange(ctx, payment, domain.PaymentStatusUnknown, reasonProviderNoAnswer, changedBy)
	}

	return s.applyStatusChange(ctx, payment, providerStatusToDomain(status), reasonSentToProvider, changedBy)
}

// Reconcile consulta al proveedor el estado real de un pago en
// PROCESSING/UNKNOWN y lo resuelve si hay una respuesta clara — pensado
// para dispararse desde fuera (ej. el worker de Python) sobre pagos
// atascados por demasiado tiempo. Si el pago ya está resuelto, lo
// devuelve tal cual, sin error.
func (s *PaymentService) Reconcile(ctx context.Context, paymentID, changedBy string) (*domain.Payment, error) {
	payment, err := s.payments.GetByID(ctx, paymentID)
	if err != nil {
		return nil, err
	}
	return s.resolveIfUncertain(ctx, payment, changedBy)
}

// reconcileWithProvider asume que payment está en PROCESSING o UNKNOWN
// (con ProviderReference/ProviderName ya asignados) y le pregunta al
// proveedor qué pasó de verdad.
func (s *PaymentService) reconcileWithProvider(ctx context.Context, payment *domain.Payment, changedBy string) (*domain.Payment, error) {
	if payment.ProviderReference == nil || payment.ProviderName == nil {
		return payment, nil // nunca se llegó a enviar a un proveedor
	}

	provider, err := s.providers.Get(*payment.ProviderName)
	if err != nil {
		return nil, err
	}

	status, err := provider.GetStatus(ctx, *payment.ProviderReference)
	if err != nil {
		if !errors.Is(err, ErrProviderUnreachable) {
			return nil, err
		}
		return s.markUnknownIfProcessing(ctx, payment, changedBy)
	}

	switch status {
	case ProviderStatusApproved:
		return s.applyStatusChange(ctx, payment, domain.PaymentStatusApproved, reasonReconciled, changedBy)
	case ProviderStatusRejected:
		return s.applyStatusChange(ctx, payment, domain.PaymentStatusRejected, reasonReconciled, changedBy)
	default: // ProviderStatusProcessing: el proveedor tampoco lo sabe todavía
		return s.markUnknownIfProcessing(ctx, payment, changedBy)
	}
}

// markUnknownIfProcessing mueve PROCESSING->UNKNOWN la primera vez que no
// hay respuesta clara; si payment ya estaba en UNKNOWN, no hay nada que
// transicionar (UNKNOWN->UNKNOWN no es una transición real).
func (s *PaymentService) markUnknownIfProcessing(ctx context.Context, payment *domain.Payment, changedBy string) (*domain.Payment, error) {
	if payment.Status != domain.PaymentStatusProcessing {
		return payment, nil
	}
	return s.applyStatusChange(ctx, payment, domain.PaymentStatusUnknown, reasonProviderNoAnswer, changedBy)
}

func providerStatusToDomain(status ProviderStatus) domain.PaymentStatus {
	if status == ProviderStatusRejected {
		return domain.PaymentStatusRejected
	}
	return domain.PaymentStatusApproved
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

// applyStatusChange aplica una transición de estado (domain.Payment.ChangeStatus
// decide si es válida), deja el registro correspondiente en el historial
// de forma atómica, e invalida la cache del resumen del comercio.
// Compartido por UpdateStatus (cambio manual) y por los cambios
// automáticos que dispara el proveedor de pagos (Create, Reconcile).
func (s *PaymentService) applyStatusChange(ctx context.Context, payment *domain.Payment, newStatus domain.PaymentStatus, reason, changedBy string) (*domain.Payment, error) {
	previousStatus := payment.Status
	if err := payment.ChangeStatus(newStatus); err != nil {
		return nil, err
	}

	// El cambio de estado y su entrada de historial deben persistirse
	// juntos o no persistirse en absoluto: si el proceso se cayera entre
	// una escritura y la otra, quedaría un pago con un estado que nadie
	// puede explicar (sin una entrada de historial que lo justifique).
	history := domain.NewPaymentStatusHistory(payment.ID, previousStatus, payment.Status, reason, changedBy)
	err := s.uow.Execute(ctx, func(txCtx context.Context) error {
		if err := s.payments.UpdateStatus(txCtx, payment); err != nil {
			return err
		}
		return s.history.Create(txCtx, history)
	})
	if err != nil {
		return nil, err
	}

	// El resumen del comercio (total/aprobados/rechazados/pendientes/monto)
	// acaba de cambiar; invalidamos la cache para que la próxima consulta
	// recalcule con datos frescos en vez de esperar a que expire el TTL.
	s.summaries.Invalidate(ctx, payment.MerchantID)

	return payment, nil
}

// UpdateStatus aplica una transición de estado manual (ej. vía
// PATCH /payments/{id}/status), sin pasar por ningún proveedor externo.
func (s *PaymentService) UpdateStatus(ctx context.Context, paymentID string, newStatus domain.PaymentStatus, reason, changedBy string) (*domain.Payment, error) {
	payment, err := s.payments.GetByID(ctx, paymentID)
	if err != nil {
		return nil, err
	}
	return s.applyStatusChange(ctx, payment, newStatus, reason, changedBy)
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

// Summary calcula (o recupera de cache) el resumen de movimientos de un
// comercio. Verifica primero que el comercio exista, para responder 404
// en vez de un resumen vacío si el ID no existe.
func (s *PaymentService) Summary(ctx context.Context, merchantID string) (*MerchantSummary, error) {
	if _, err := s.merchants.GetByID(ctx, merchantID); err != nil {
		return nil, err
	}

	if cached, ok := s.summaries.Get(ctx, merchantID); ok {
		return &cached, nil
	}

	summary, err := s.payments.GetSummaryByMerchantID(ctx, merchantID)
	if err != nil {
		return nil, err
	}

	s.summaries.Set(ctx, merchantID, summary)
	return &summary, nil
}

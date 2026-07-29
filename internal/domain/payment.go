package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type PaymentStatus string

const (
	PaymentStatusPending    PaymentStatus = "PENDING"
	PaymentStatusProcessing PaymentStatus = "PROCESSING"
	PaymentStatusApproved   PaymentStatus = "APPROVED"
	PaymentStatusRejected   PaymentStatus = "REJECTED"
	PaymentStatusCancelled  PaymentStatus = "CANCELLED"
	// PaymentStatusUnknown representa una operación que se envió a un
	// proveedor externo pero cuyo resultado no se pudo confirmar (caída de
	// red, timeout): no es un estado terminal, exige conciliación con el
	// proveedor para resolverse a APPROVED o REJECTED.
	PaymentStatusUnknown PaymentStatus = "UNKNOWN"
)

func (s PaymentStatus) IsValid() bool {
	switch s {
	case PaymentStatusPending, PaymentStatusProcessing, PaymentStatusApproved,
		PaymentStatusRejected, PaymentStatusCancelled, PaymentStatusUnknown:
		return true
	default:
		return false
	}
}

type PaymentMethod string

const (
	PaymentMethodCard     PaymentMethod = "CARD"
	PaymentMethodQR       PaymentMethod = "QR"
	PaymentMethodNFC      PaymentMethod = "NFC"
	PaymentMethodTransfer PaymentMethod = "TRANSFER"
)

func (m PaymentMethod) IsValid() bool {
	switch m {
	case PaymentMethodCard, PaymentMethodQR, PaymentMethodNFC, PaymentMethodTransfer:
		return true
	default:
		return false
	}
}

// allowedTransitions codifica las únicas transiciones de estado válidas.
//
// PENDING conserva su salida directa a APPROVED/REJECTED/CANCELLED (el
// cambio manual vía PATCH /payments/{id}/status, sin pasar por ningún
// proveedor externo). PROCESSING y UNKNOWN son el camino que sigue el
// flujo automático de creación cuando sí hay un proveedor externo de por
// medio: PENDING → PROCESSING se persiste ANTES de llamar al proveedor
// (para que una caída a mitad de la llamada deje evidencia de que la
// operación quedó en camino, no perdida); desde ahí, una respuesta clara
// del proveedor mueve a APPROVED/REJECTED, y la ausencia de respuesta
// mueve a UNKNOWN. UNKNOWN solo se resuelve conciliando con el proveedor
// (nunca con una cancelación manual — cancelar algo que en realidad sí
// se cobró falsearía el registro).
var allowedTransitions = map[PaymentStatus][]PaymentStatus{
	PaymentStatusPending: {
		PaymentStatusProcessing,
		PaymentStatusApproved,
		PaymentStatusRejected,
		PaymentStatusCancelled,
	},
	PaymentStatusProcessing: {
		PaymentStatusApproved,
		PaymentStatusRejected,
		PaymentStatusUnknown,
	},
	PaymentStatusUnknown: {
		PaymentStatusApproved,
		PaymentStatusRejected,
	},
}

func CanTransition(from, to PaymentStatus) bool {
	for _, allowed := range allowedTransitions[from] {
		if allowed == to {
			return true
		}
	}
	return false
}

type Payment struct {
	ID                string
	MerchantID        string
	ExternalReference string
	Amount            Money
	PaymentMethod     PaymentMethod
	Status            PaymentStatus
	IdempotencyKey    string
	// ProviderReference es el identificador que asigna el proveedor
	// externo (ej. Nequi) a la operación de cobro; nil hasta que el pago
	// se envía al proveedor (ver PaymentProvider en application/ports.go).
	ProviderReference *string
	// ProviderName identifica CUÁL proveedor externo procesó el pago (ej.
	// "nequi", "bre-b", "simulated") — permite soportar más de un
	// proveedor sin ambigüedad sobre a cuál conciliar. También nil hasta
	// que el pago se envía a un proveedor.
	ProviderName *string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// NewPayment valida los campos exigidos por las reglas de negocio y crea
// un Payment en estado PENDING. Se espera que la llave de idempotencia la
// genere quien llama (el cliente de la API), no esta función.
func NewPayment(merchantID, externalReference string, amount decimal.Decimal, currency string, method PaymentMethod, idempotencyKey string) (*Payment, error) {
	if merchantID == "" {
		return nil, ErrMissingMerchantID
	}
	if externalReference == "" {
		return nil, ErrMissingExternalReference
	}
	if !method.IsValid() {
		return nil, ErrInvalidPaymentMethod
	}
	if idempotencyKey == "" {
		return nil, ErrMissingIdempotencyKey
	}

	money, err := NewMoney(amount, currency)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	return &Payment{
		ID:                 uuid.New().String(),
		MerchantID:         merchantID,
		ExternalReference:  externalReference,
		Amount:             money,
		PaymentMethod:      method,
		Status:             PaymentStatusPending,
		IdempotencyKey:     idempotencyKey,
		CreatedAt:          now,
		UpdatedAt:          now,
	}, nil
}

// ChangeStatus aplica la tabla de transiciones permitidas antes de mutar
// el pago. Quien llama es responsable de registrar el PaymentStatusHistory
// resultante.
func (p *Payment) ChangeStatus(newStatus PaymentStatus) error {
	if !newStatus.IsValid() {
		return ErrInvalidPaymentStatus
	}
	if !CanTransition(p.Status, newStatus) {
		return ErrInvalidStatusTransition
	}
	p.Status = newStatus
	p.UpdatedAt = time.Now().UTC()
	return nil
}
package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type PaymentStatus string

const (
	PaymentStatusPending   PaymentStatus = "PENDING"
	PaymentStatusApproved  PaymentStatus = "APPROVED"
	PaymentStatusRejected  PaymentStatus = "REJECTED"
	PaymentStatusCancelled PaymentStatus = "CANCELLED"
)

func (s PaymentStatus) IsValid() bool {
	switch s {
	case PaymentStatusPending, PaymentStatusApproved, PaymentStatusRejected, PaymentStatusCancelled:
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

// allowedTransitions codifica las únicas transiciones de estado válidas:
// un pago sale de PENDING una sola vez, hacia cualquier estado terminal,
// y los estados terminales nunca vuelven a transicionar.
var allowedTransitions = map[PaymentStatus][]PaymentStatus{
	PaymentStatusPending: {PaymentStatusApproved, PaymentStatusRejected, PaymentStatusCancelled},
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
	ID                 string
	MerchantID         string
	ExternalReference  string
	Amount             Money
	PaymentMethod      PaymentMethod
	Status             PaymentStatus
	IdempotencyKey     string
	CreatedAt          time.Time
	UpdatedAt          time.Time
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
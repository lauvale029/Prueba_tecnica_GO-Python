package domain

import (
	"time"

	"github.com/google/uuid"
)

// PaymentStatusHistory registra un cambio de estado puntual de un pago
type PaymentStatusHistory struct {
	ID             string
	PaymentID      string
	PreviousStatus PaymentStatus
	NewStatus      PaymentStatus
	Reason         string
	ChangedBy      string
	CreatedAt      time.Time
}

func NewPaymentStatusHistory(paymentID string, previous, newStatus PaymentStatus, reason, changedBy string) *PaymentStatusHistory {
	return &PaymentStatusHistory{
		ID:             uuid.New().String(),
		PaymentID:      paymentID,
		PreviousStatus: previous,
		NewStatus:      newStatus,
		Reason:         reason,
		ChangedBy:      changedBy,
		CreatedAt:      time.Now().UTC(),
	}
}

package domain_test

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/domain"
)

func newValidPayment(t *testing.T) *domain.Payment {
	t.Helper()
	p, err := domain.NewPayment("merchant-1", "ORDER-1", decimal.NewFromInt(1000), "COP", domain.PaymentMethodQR, "key-1")
	require.NoError(t, err)
	return p
}

func TestNewPayment_Valid(t *testing.T) {
	p := newValidPayment(t)

	require.NotEmpty(t, p.ID)
	require.Equal(t, domain.PaymentStatusPending, p.Status)
	require.Equal(t, "COP", p.Amount.Currency)
}

func TestNewPayment_InvalidAmount(t *testing.T) {
	_, err := domain.NewPayment("merchant-1", "ORDER-1", decimal.Zero, "COP", domain.PaymentMethodQR, "key-1")
	require.ErrorIs(t, err, domain.ErrInvalidAmount)
}

func TestNewPayment_InvalidPaymentMethod(t *testing.T) {
	_, err := domain.NewPayment("merchant-1", "ORDER-1", decimal.NewFromInt(1000), "COP", domain.PaymentMethod("INVALID"), "key-1")
	require.ErrorIs(t, err, domain.ErrInvalidPaymentMethod)
}

func TestNewPayment_MissingIdempotencyKey(t *testing.T) {
	_, err := domain.NewPayment("merchant-1", "ORDER-1", decimal.NewFromInt(1000), "COP", domain.PaymentMethodQR, "")
	require.ErrorIs(t, err, domain.ErrMissingIdempotencyKey)
}

func TestNewPayment_MissingExternalReference(t *testing.T) {
	_, err := domain.NewPayment("merchant-1", "", decimal.NewFromInt(1000), "COP", domain.PaymentMethodQR, "key-1")
	require.ErrorIs(t, err, domain.ErrMissingExternalReference)
}

func TestCanTransition(t *testing.T) {
	tests := []struct {
		from domain.PaymentStatus
		to   domain.PaymentStatus
		want bool
	}{
		{domain.PaymentStatusPending, domain.PaymentStatusApproved, true},
		{domain.PaymentStatusPending, domain.PaymentStatusRejected, true},
		{domain.PaymentStatusPending, domain.PaymentStatusCancelled, true},
		{domain.PaymentStatusApproved, domain.PaymentStatusPending, false},
		{domain.PaymentStatusApproved, domain.PaymentStatusRejected, false},
		{domain.PaymentStatusApproved, domain.PaymentStatusCancelled, false},
		{domain.PaymentStatusRejected, domain.PaymentStatusApproved, false},
		{domain.PaymentStatusRejected, domain.PaymentStatusPending, false},
		{domain.PaymentStatusCancelled, domain.PaymentStatusPending, false},
	}

	for _, tt := range tests {
		got := domain.CanTransition(tt.from, tt.to)
		require.Equal(t, tt.want, got, "from=%s to=%s", tt.from, tt.to)
	}
}

func TestPayment_ChangeStatus_Valid(t *testing.T) {
	p := newValidPayment(t)

	err := p.ChangeStatus(domain.PaymentStatusApproved)

	require.NoError(t, err)
	require.Equal(t, domain.PaymentStatusApproved, p.Status)
}

func TestPayment_ChangeStatus_InvalidTransition(t *testing.T) {
	p := newValidPayment(t)
	require.NoError(t, p.ChangeStatus(domain.PaymentStatusApproved))

	err := p.ChangeStatus(domain.PaymentStatusRejected)

	require.ErrorIs(t, err, domain.ErrInvalidStatusTransition)
}

func TestPayment_ChangeStatus_CannotReturnToPending(t *testing.T) {
	p := newValidPayment(t)
	require.NoError(t, p.ChangeStatus(domain.PaymentStatusCancelled))

	err := p.ChangeStatus(domain.PaymentStatusPending)

	require.ErrorIs(t, err, domain.ErrInvalidStatusTransition)
}
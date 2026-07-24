package domain_test

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/domain"
)

func TestNewMoney(t *testing.T) {
	tests := []struct {
		name     string
		amount   decimal.Decimal
		currency string
		wantErr  error
	}{
		{"valid amount and currency", decimal.NewFromInt(1000), "COP", nil},
		{"zero amount", decimal.Zero, "COP", domain.ErrInvalidAmount},
		{"negative amount", decimal.NewFromInt(-100), "COP", domain.ErrInvalidAmount},
		{"unsupported currency", decimal.NewFromInt(1000), "USD", domain.ErrUnsupportedCurrency},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := domain.NewMoney(tt.amount, tt.currency)
			if tt.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}
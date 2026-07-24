package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/domain"
)

func TestNewMerchant_Valid(t *testing.T) {
	m, err := domain.NewMerchant("Comercio Prueba", "900123456", "comercio@example.com")

	require.NoError(t, err)
	require.NotEmpty(t, m.ID)
	require.Equal(t, domain.MerchantStatusActive, m.Status)
}

func TestNewMerchant_MissingName(t *testing.T) {
	_, err := domain.NewMerchant("", "900123456", "comercio@example.com")
	require.ErrorIs(t, err, domain.ErrMissingMerchantName)
}

func TestNewMerchant_MissingDocumentNumber(t *testing.T) {
	_, err := domain.NewMerchant("Comercio Prueba", "", "comercio@example.com")
	require.ErrorIs(t, err, domain.ErrMissingDocumentNumber)
}

func TestNewMerchant_InvalidEmail(t *testing.T) {
	_, err := domain.NewMerchant("Comercio Prueba", "900123456", "not-an-email")
	require.ErrorIs(t, err, domain.ErrInvalidEmail)
}
package domain

import (
	"net/mail"
	"time"

	"github.com/google/uuid"
)

type MerchantStatus string

const (
	MerchantStatusActive   MerchantStatus = "ACTIVE"
	MerchantStatusInactive MerchantStatus = "INACTIVE"
)

type Merchant struct {
	ID             string
	Name           string
	DocumentNumber string
	Email          string
	Status         MerchantStatus
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// NewMerchant valida los campos obligatorios y produce un Merchant listo
// para persistir, siempre iniciando en estado ACTIVE.
func NewMerchant(name, documentNumber, email string) (*Merchant, error) {
	if name == "" {
		return nil, ErrMissingMerchantName
	}
	if documentNumber == "" {
		return nil, ErrMissingDocumentNumber
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return nil, ErrInvalidEmail
	}

	now := time.Now().UTC()
	return &Merchant{
		ID:             uuid.New().String(),
		Name:           name,
		DocumentNumber: documentNumber,
		Email:          email,
		Status:         MerchantStatusActive,
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}
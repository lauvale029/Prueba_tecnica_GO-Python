package domain

import "errors"

// Errores de validación y de reglas de negocio a nivel de dominio. Las
// capas de transporte traducen estos errores a los códigos HTTP
// correspondientes (400/409/422).
var (
	ErrInvalidAmount            = errors.New("el monto debe ser mayor que cero")
	ErrUnsupportedCurrency      = errors.New("moneda no soportada")
	ErrInvalidPaymentMethod     = errors.New("medio de pago inválido")
	ErrInvalidPaymentStatus     = errors.New("estado de pago inválido")
	ErrInvalidStatusTransition  = errors.New("la transición de estado del pago no está permitida")
	ErrMissingMerchantID        = errors.New("el id del comercio es obligatorio")
	ErrMissingMerchantName      = errors.New("el nombre del comercio es obligatorio")
	ErrMissingDocumentNumber    = errors.New("el número de documento del comercio es obligatorio")
	ErrInvalidEmail             = errors.New("el email no es válido")
	ErrMissingExternalReference = errors.New("la referencia externa es obligatoria")
	ErrMissingIdempotencyKey    = errors.New("la llave de idempotencia es obligatoria")
)
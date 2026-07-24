package domain

import "github.com/shopspring/decimal"

// SupportedCurrency es la única moneda aceptada por ahora. Ver "Supuestos"
// en el README para la justificación.
const SupportedCurrency = "COP"

// Money es un value object que agrupa un monto con su moneda. Siempre se
// construye a través de NewMoney, así nunca puede existir un Money
// inválido dentro del dominio.
type Money struct {
	Amount   decimal.Decimal
	Currency string
}

func NewMoney(amount decimal.Decimal, currency string) (Money, error) {
	if amount.LessThanOrEqual(decimal.Zero) {
		return Money{}, ErrInvalidAmount
	}
	if currency != SupportedCurrency {
		return Money{}, ErrUnsupportedCurrency
	}
	return Money{Amount: amount, Currency: currency}, nil
}
package application

import "errors"

// Errores genéricos del contrato de repositorio (los "puertos" en
// ports.go). Cualquier implementación (Postgres, un fake de test, u otra
// base de datos) debe devolver estos cuando corresponda, para que
// application y transport/http no necesiten conocer detalles de la
// infraestructura concreta que hay detrás.
var (
	// ErrNotFound se devuelve cuando el recurso solicitado no existe.
	ErrNotFound = errors.New("recurso no encontrado")
	// ErrConflict se devuelve cuando la operación viola una restricción
	// de unicidad (ej. document_number, idempotency_key o
	// merchant_id+external_reference duplicados).
	ErrConflict = errors.New("conflicto: el recurso ya existe")
)
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

	// ErrUnknownProvider lo devuelve un ProviderRegistry (ver ports.go)
	// cuando se pide un proveedor con un nombre que nadie registró — un
	// error de configuración, no una respuesta del proveedor en sí.
	ErrUnknownProvider = errors.New("proveedor de pagos no configurado")

	// ErrProviderUnreachable lo devuelve un PaymentProvider (ver ports.go)
	// cuando no se pudo obtener ninguna respuesta suya (timeout, caída de
	// red) — a diferencia de una respuesta real de rechazo, esto no dice
	// nada sobre si la operación se procesó o no del lado del proveedor;
	// por eso PaymentService lo trata como estado incierto (UNKNOWN), no
	// como un fallo definitivo.
	ErrProviderUnreachable = errors.New("no se pudo contactar al proveedor de pagos")
)
package postgres

import (
	"context"
	"database/sql"

	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/infrastructure/postgres/sqlcgen"
)

// txContextKey es una clave privada al paquete a propósito: nadie fuera
// de internal/infrastructure/postgres puede leer ni escribir el valor,
// así que la única forma de que un ctx traiga una transacción es haber
// pasado por UnitOfWork.Execute (ver unit_of_work.go).
type txContextKey struct{}

func withTx(ctx context.Context, tx *sql.Tx) context.Context {
	return context.WithValue(ctx, txContextKey{}, tx)
}

// dbtxFromContext devuelve la transacción activa si ctx trae una (porque
// esta llamada ocurre dentro de un UnitOfWork.Execute), o fallback (la
// conexión normal, sin transacción) si no. Es lo que le permite a un
// mismo repositorio escribir tanto dentro como fuera de una transacción,
// sin que quien lo llama tenga que saber cuál es cuál.
func dbtxFromContext(ctx context.Context, fallback sqlcgen.DBTX) sqlcgen.DBTX {
	if tx, ok := ctx.Value(txContextKey{}).(*sql.Tx); ok {
		return tx
	}
	return fallback
}

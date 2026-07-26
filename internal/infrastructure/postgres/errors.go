package postgres

import (
	"database/sql"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/application"
)

// mapError traduce errores concretos de Postgres/database-sql a los
// errores genéricos definidos en application (ErrNotFound/ErrConflict),
// para que las capas superiores no necesiten conocer detalles de la
// base de datos subyacente. postgres es el adaptador: depende de
// application (el puerto), nunca al revés.
func mapError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return application.ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return application.ErrConflict
	}
	return err
}
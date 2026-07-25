package postgres

import (
	"database/sql"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

var (
	// ErrNotFound se devuelve cuando una consulta no encuentra filas.
	ErrNotFound = errors.New("recurso no encontrado")
	// ErrConflict se devuelve cuando una restricción única de la base de
	// datos rechaza la operación (código de Postgres 23505).
	ErrConflict = errors.New("conflicto: el recurso ya existe")
)

// mapError traduce errores concretos de Postgres/database-sql a los
// errores genéricos de arriba, para que las capas superiores no necesiten
// conocer detalles de la base de datos subyacente.
func mapError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrConflict
	}
	return err
}
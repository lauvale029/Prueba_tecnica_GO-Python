package postgres

import (
	"database/sql"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// NewDB abre un *sql.DB usando pgx registrado como driver de database/sql
// (ver decisión técnica en el README: sqlc + database/sql, no el modo
// nativo de pgx).
func NewDB(databaseURL string) (*sql.DB, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}
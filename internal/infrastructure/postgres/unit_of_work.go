package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/application"
)

// UnitOfWork implementa application.UnitOfWork con una transacción real
// de Postgres. Ver tx.go para cómo los repositorios de este paquete
// detectan y usan la transacción que Execute deja en el contexto.
type UnitOfWork struct {
	db *sql.DB
}

func NewUnitOfWork(db *sql.DB) *UnitOfWork {
	return &UnitOfWork{db: db}
}

var _ application.UnitOfWork = (*UnitOfWork)(nil)

func (u *UnitOfWork) Execute(ctx context.Context, fn func(ctx context.Context) error) error {
	tx, err := u.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("iniciando transacción: %w", err)
	}

	if err := fn(withTx(ctx, tx)); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("revirtiendo transacción tras error (%v): %w", err, rbErr)
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("confirmando transacción: %w", err)
	}
	return nil
}

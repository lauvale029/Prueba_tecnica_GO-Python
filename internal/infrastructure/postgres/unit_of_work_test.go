//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/domain"
	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/infrastructure/postgres"
)

// TestUnitOfWork_RollsBackAllWritesOnError es la prueba central de la
// atomicidad: si la segunda escritura de la transacción falla, la primera
// (que ya se había ejecutado con éxito) debe revertirse también — Postgres
// nunca debe quedar con un pago en un estado a medio camino.
func TestUnitOfWork_RollsBackAllWritesOnError(t *testing.T) {
	db := testDB(t)
	merchant := createTestMerchant(t, db)
	paymentRepo := postgres.NewPaymentRepository(db)
	historyRepo := postgres.NewPaymentStatusHistoryRepository(db)
	uow := postgres.NewUnitOfWork(db)
	ctx := context.Background()

	payment := newTestPayment(t, merchant.ID, "key-"+uuid.New().String())
	require.NoError(t, paymentRepo.Create(ctx, payment))

	require.NoError(t, payment.ChangeStatus(domain.PaymentStatusApproved))

	forcedErr := errors.New("fallo simulado a mitad de la transacción")
	err := uow.Execute(ctx, func(txCtx context.Context) error {
		// Primera escritura: sí se ejecuta con éxito dentro de la
		// transacción.
		if err := paymentRepo.UpdateStatus(txCtx, payment); err != nil {
			return err
		}
		// Segunda escritura: nunca llega a persistirse, porque forzamos
		// un error ANTES de intentarla — simula, por ejemplo, que
		// domain.NewPaymentStatusHistory o Create fallaran por cualquier
		// motivo a mitad de camino.
		return forcedErr
	})
	require.ErrorIs(t, err, forcedErr)

	// La pregunta que importa: ¿el UPDATE de arriba, que sí se ejecutó
	// sin error, quedó revertido? Si Execute funciona bien, la respuesta
	// tiene que ser sí.
	found, getErr := paymentRepo.GetByID(ctx, payment.ID)
	require.NoError(t, getErr)
	require.Equal(t, domain.PaymentStatusPending, found.Status,
		"el cambio de estado debió revertirse junto con el resto de la transacción")

	history, histErr := historyRepo.ListByPaymentID(ctx, payment.ID)
	require.NoError(t, histErr)
	require.Empty(t, history, "no debería haber quedado ninguna entrada de historial huérfana")
}

// TestUnitOfWork_CommitsAllWritesOnSuccess confirma el camino feliz: si
// fn no devuelve error, ambas escrituras quedan persistidas juntas.
func TestUnitOfWork_CommitsAllWritesOnSuccess(t *testing.T) {
	db := testDB(t)
	merchant := createTestMerchant(t, db)
	paymentRepo := postgres.NewPaymentRepository(db)
	historyRepo := postgres.NewPaymentStatusHistoryRepository(db)
	uow := postgres.NewUnitOfWork(db)
	ctx := context.Background()

	payment := newTestPayment(t, merchant.ID, "key-"+uuid.New().String())
	require.NoError(t, paymentRepo.Create(ctx, payment))

	previousStatus := payment.Status
	require.NoError(t, payment.ChangeStatus(domain.PaymentStatusApproved))
	history := domain.NewPaymentStatusHistory(payment.ID, previousStatus, payment.Status, "aprobado en test", "test-suite")

	err := uow.Execute(ctx, func(txCtx context.Context) error {
		if err := paymentRepo.UpdateStatus(txCtx, payment); err != nil {
			return err
		}
		return historyRepo.Create(txCtx, history)
	})
	require.NoError(t, err)

	found, err := paymentRepo.GetByID(ctx, payment.ID)
	require.NoError(t, err)
	require.Equal(t, domain.PaymentStatusApproved, found.Status)

	entries, err := historyRepo.ListByPaymentID(ctx, payment.ID)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, domain.PaymentStatusApproved, entries[0].NewStatus)
}

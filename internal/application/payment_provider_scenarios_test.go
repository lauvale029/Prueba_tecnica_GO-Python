// Este archivo contiene los 5 escenarios de riesgo pedidos para la
// resiliencia frente al proveedor externo de pagos (ver README, Sección
// 2), narrados paso a paso con t.Logf — corre con:
//
//	go test ./internal/application/... -run TestScenario -v
//
// (el -v es necesario para que Go muestre los t.Logf incluso cuando el
// test pasa; sin -v, solo se ven si el test falla).
package application_test

import (
	"context"
	"sync"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/domain"
)

// TestScenario1_NormalApproval — el proveedor responde de inmediato,
// sin ningún contratiempo.
func TestScenario1_NormalApproval(t *testing.T) {
	service, merchants, _, history, _, _ := newPaymentServiceWithProvider(fakeProviderApprove)
	merchant := seedMerchant(t, merchants)

	t.Log("1. Un comercio inicia un pago de $150.000 COP")
	payment, err := service.Create(context.Background(), merchant.ID, "ORDER-ESCENARIO-1",
		decimal.NewFromInt(150000), "COP", domain.PaymentMethodQR, "key-escenario-1", "mova-service")
	require.NoError(t, err)

	t.Logf("2. MOVA generó la referencia %s y envió el cobro al proveedor", *payment.ProviderReference)
	t.Log("3. El proveedor respondió de inmediato: APROBADO")
	require.Equal(t, domain.PaymentStatusApproved, payment.Status)
	t.Logf("4. Resultado final: pago %s queda en %s", payment.ID, payment.Status)

	entries, err := history.ListByPaymentID(context.Background(), payment.ID)
	require.NoError(t, err)
	t.Log("5. Historial completo de este pago:")
	for _, e := range entries {
		t.Logf("   %s -> %s (%s)", e.PreviousStatus, e.NewStatus, e.Reason)
	}
	require.Len(t, entries, 2, "PENDING->PROCESSING y PROCESSING->APPROVED")
}

// TestScenario2_TimeoutBeforeKnowingResult — el proveedor nunca
// responde, y al intentar conciliar de inmediato, tampoco sabe nada
// todavía (a diferencia del escenario 4, acá NO hay una verdad oculta
// que revelar — el proveedor genuinamente no la tiene).
func TestScenario2_TimeoutBeforeKnowingResult(t *testing.T) {
	service, merchants, _, _, _, _ := newPaymentServiceWithProvider(fakeProviderUnreachable)
	merchant := seedMerchant(t, merchants)

	t.Log("1. Un comercio inicia un pago, pero el proveedor no responde a tiempo (timeout)")
	payment, err := service.Create(context.Background(), merchant.ID, "ORDER-ESCENARIO-2",
		decimal.NewFromInt(150000), "COP", domain.PaymentMethodQR, "key-escenario-2", "mova-service")
	require.NoError(t, err)

	t.Logf("2. MOVA no recibió ninguna respuesta del proveedor")
	require.Equal(t, domain.PaymentStatusUnknown, payment.Status)
	require.NotNil(t, payment.ProviderReference, "la referencia se guardó ANTES de llamar, así que no se pierde")
	t.Logf("3. El pago queda en estado incierto: %s (referencia %s a salvo)", payment.Status, *payment.ProviderReference)

	t.Log("4. Intentamos conciliar ya mismo — el proveedor TAMPOCO sabe nada todavía")
	reconciled, err := service.Reconcile(context.Background(), payment.ID, "worker-conciliacion")
	require.NoError(t, err)
	t.Logf("5. Sigue en %s — habrá que reintentar la conciliación más tarde", reconciled.Status)
	require.Equal(t, domain.PaymentStatusUnknown, reconciled.Status)
}

// TestScenario3_RetrySameIdempotencyKey — el cliente, sin saber si su
// primer intento funcionó, reintenta con la MISMA Idempotency-Key. La
// prueba clave: el proveedor solo recibe UN cobro real, nunca dos.
func TestScenario3_RetrySameIdempotencyKey(t *testing.T) {
	service, merchants, _, _, _, provider := newPaymentServiceWithProvider(fakeProviderApprove)
	merchant := seedMerchant(t, merchants)
	key := "key-escenario-3"

	t.Log("1. Primer intento de pago")
	first, err := service.Create(context.Background(), merchant.ID, "ORDER-ESCENARIO-3",
		decimal.NewFromInt(150000), "COP", domain.PaymentMethodQR, key, "mova-service")
	require.NoError(t, err)
	t.Logf("   -> pago %s creado en %s (provider.Charge llamado %d vez)", first.ID, first.Status, provider.chargeCalls())

	t.Log("2. El cliente, sin saber si funcionó, reintenta con LA MISMA Idempotency-Key")
	second, err := service.Create(context.Background(), merchant.ID, "ORDER-ESCENARIO-3",
		decimal.NewFromInt(150000), "COP", domain.PaymentMethodQR, key, "mova-service")
	require.NoError(t, err)

	t.Logf("3. Se devolvió el pago %s (el mismo: %v)", second.ID, first.ID == second.ID)
	require.Equal(t, first.ID, second.ID, "el reintento nunca debe crear un pago nuevo")

	t.Logf("4. provider.Charge se llamó %d vez(es) en total — el reintento NO volvió a cobrar", provider.chargeCalls())
	require.Equal(t, 1, provider.chargeCalls(), "un reintento con la misma key jamás debe generar un segundo cobro")
}

// TestScenario4_ApprovedButResponseLost — el escenario de riesgo
// central: el proveedor SÍ aprueba de verdad, pero la respuesta hacia
// MOVA se pierde antes de guardarse. Se resuelve conciliando.
func TestScenario4_ApprovedButResponseLost(t *testing.T) {
	service, merchants, _, _, _, _ := newPaymentServiceWithProvider(fakeProviderApprovedButLost)
	merchant := seedMerchant(t, merchants)

	t.Log("1. El proveedor SÍ aprueba el cobro de verdad — pero la respuesta hacia MOVA se pierde")
	payment, err := service.Create(context.Background(), merchant.ID, "ORDER-ESCENARIO-4",
		decimal.NewFromInt(150000), "COP", domain.PaymentMethodQR, "key-escenario-4", "mova-service")
	require.NoError(t, err)

	t.Logf("2. MOVA no se enteró de la aprobación real: el pago quedó en %s (no en APPROVED)", payment.Status)
	require.Equal(t, domain.PaymentStatusUnknown, payment.Status)

	t.Log("3. Se dispara la conciliación (ej. el worker de Python, o un reintento del cliente)")
	reconciled, err := service.Reconcile(context.Background(), payment.ID, "worker-conciliacion")
	require.NoError(t, err)

	t.Logf("4. El proveedor revela lo que sabía desde el principio: %s", reconciled.Status)
	require.Equal(t, domain.PaymentStatusApproved, reconciled.Status, "la conciliación debe revelar la aprobación real")
}

// TestScenario5_ConcurrentRequests — dos peticiones llegan exactamente
// al mismo tiempo con la MISMA Idempotency-Key. Ninguna debe duplicar el
// pago, ni cobrar dos veces.
func TestScenario5_ConcurrentRequests(t *testing.T) {
	service, merchants, payments, _, _, provider := newPaymentServiceWithProvider(fakeProviderApprove)
	merchant := seedMerchant(t, merchants)
	key := "key-escenario-5"

	t.Log("1. Dos peticiones llegan AL MISMO TIEMPO con la misma Idempotency-Key")
	const attempts = 2
	var wg sync.WaitGroup
	results := make([]*domain.Payment, attempts)
	errs := make([]error, attempts)

	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = service.Create(context.Background(), merchant.ID, "ORDER-ESCENARIO-5",
				decimal.NewFromInt(150000), "COP", domain.PaymentMethodQR, key, "mova-service")
		}(i)
	}
	wg.Wait()

	require.NoError(t, errs[0])
	require.NoError(t, errs[1])
	t.Logf("2. Petición A -> pago %s (%s)", results[0].ID, results[0].Status)
	t.Logf("3. Petición B -> pago %s (%s)", results[1].ID, results[1].Status)

	require.Equal(t, results[0].ID, results[1].ID, "ambas deben devolver el MISMO pago")
	t.Log("4. Ambas devolvieron el mismo pago — no se creó ninguna fila duplicada")
	require.Equal(t, 1, payments.rowCount())

	t.Logf("5. provider.Charge se llamó %d vez(es) en total — nunca se cobró dos veces", provider.chargeCalls())
	require.Equal(t, 1, provider.chargeCalls())
}

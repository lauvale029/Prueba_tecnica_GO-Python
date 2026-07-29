package provider_test

import (
	"context"
	"sync"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/application"
	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/domain"
	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/infrastructure/provider"
)

func testChargeRequest(reference string) application.ChargeRequest {
	return application.ChargeRequest{
		ProviderReference: reference,
		Amount:            decimal.NewFromInt(150000),
		Currency:          "COP",
		PaymentMethod:     domain.PaymentMethodCard,
	}
}

func TestSimulatedProvider_Approve(t *testing.T) {
	p := provider.NewSimulatedProvider(provider.BehaviorApprove)

	status, err := p.Charge(context.Background(), testChargeRequest("ref-1"))
	require.NoError(t, err)
	require.Equal(t, application.ProviderStatusApproved, status)

	// GetStatus debe coincidir con lo que Charge ya reportó.
	status, err = p.GetStatus(context.Background(), "ref-1")
	require.NoError(t, err)
	require.Equal(t, application.ProviderStatusApproved, status)
}

func TestSimulatedProvider_Reject(t *testing.T) {
	p := provider.NewSimulatedProvider(provider.BehaviorReject)

	status, err := p.Charge(context.Background(), testChargeRequest("ref-2"))
	require.NoError(t, err)
	require.Equal(t, application.ProviderStatusRejected, status)
}

func TestSimulatedProvider_Timeout_ChargeFails(t *testing.T) {
	p := provider.NewSimulatedProvider(provider.BehaviorTimeout)

	_, err := p.Charge(context.Background(), testChargeRequest("ref-3"))
	require.ErrorIs(t, err, application.ErrProviderUnreachable)
}

func TestSimulatedProvider_Timeout_GetStatusStaysProcessingUntilResolved(t *testing.T) {
	p := provider.NewSimulatedProvider(provider.BehaviorTimeout)
	_, _ = p.Charge(context.Background(), testChargeRequest("ref-4"))

	status, err := p.GetStatus(context.Background(), "ref-4")
	require.NoError(t, err)
	require.Equal(t, application.ProviderStatusProcessing, status, "el proveedor tampoco sabe todavía, no es un error")

	// Con el tiempo, el proveedor sí termina de resolverlo.
	p.Resolve("ref-4", application.ProviderStatusApproved)

	status, err = p.GetStatus(context.Background(), "ref-4")
	require.NoError(t, err)
	require.Equal(t, application.ProviderStatusApproved, status)
}

func TestSimulatedProvider_ApprovedButLost_ChargeFailsButGetStatusRevealsApproval(t *testing.T) {
	p := provider.NewSimulatedProvider(provider.BehaviorApprovedButLost)

	_, err := p.Charge(context.Background(), testChargeRequest("ref-5"))
	require.ErrorIs(t, err, application.ErrProviderUnreachable, "la respuesta hacia nosotros se simula perdida")

	status, err := p.GetStatus(context.Background(), "ref-5")
	require.NoError(t, err)
	require.Equal(t, application.ProviderStatusApproved, status, "el proveedor sí lo aprobó de verdad, aunque nunca nos llegó la respuesta")
}

func TestSimulatedProvider_GetStatus_UnknownReferenceIsProcessing(t *testing.T) {
	p := provider.NewSimulatedProvider(provider.BehaviorApprove)

	status, err := p.GetStatus(context.Background(), "referencia-que-nunca-se-cobro")
	require.NoError(t, err)
	require.Equal(t, application.ProviderStatusProcessing, status)
}

func TestSimulatedProvider_ConcurrentCharges_AreSafe(t *testing.T) {
	p := provider.NewSimulatedProvider(provider.BehaviorApprove)

	const attempts = 20
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ref := "ref-concurrent"
			_, _ = p.Charge(context.Background(), testChargeRequest(ref))
			_, _ = p.GetStatus(context.Background(), ref)
		}(i)
	}
	wg.Wait()
	// El objetivo de este test es que -race no detecte ninguna carrera
	// sobre el mapa interno; si el Mutex faltara, `go test -race` fallaría.
}

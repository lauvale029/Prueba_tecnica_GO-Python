package provider_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/application"
	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/infrastructure/provider"
)

func TestRegistry_GetRegisteredProvider(t *testing.T) {
	nequi := provider.NewSimulatedProvider(provider.BehaviorApprove)
	registry := provider.NewRegistry().Register("nequi", nequi)

	got, err := registry.Get("nequi")
	require.NoError(t, err)
	require.Same(t, nequi, got)
}

func TestRegistry_UnknownProviderReturnsError(t *testing.T) {
	registry := provider.NewRegistry().Register("nequi", provider.NewSimulatedProvider(provider.BehaviorApprove))

	_, err := registry.Get("bre-b")
	require.ErrorIs(t, err, application.ErrUnknownProvider)
}

func TestRegistry_SupportsMultipleProvidersAtOnce(t *testing.T) {
	nequi := provider.NewSimulatedProvider(provider.BehaviorApprove)
	brebSimulated := provider.NewSimulatedProvider(provider.BehaviorReject)

	registry := provider.NewRegistry().
		Register("nequi", nequi).
		Register("bre-b", brebSimulated)

	gotNequi, err := registry.Get("nequi")
	require.NoError(t, err)
	require.Same(t, nequi, gotNequi)

	gotBreb, err := registry.Get("bre-b")
	require.NoError(t, err)
	require.Same(t, brebSimulated, gotBreb)
}

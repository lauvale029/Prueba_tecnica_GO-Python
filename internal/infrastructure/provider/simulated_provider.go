// Package provider contiene un simulador de proveedor externo de pagos
// (tipo Nequi/Bre-B), usado en ausencia de credenciales reales de sandbox.
// Implementa application.PaymentProvider con comportamientos configurables
// para poder reproducir, de forma determinista, los escenarios de riesgo
// descritos en el README (aprobación normal, timeout, y la respuesta
// aprobada que se pierde antes de llegar a MOVA).
package provider

import (
	"context"
	"sync"

	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/application"
)

// Behavior decide qué le pasa a cada llamada a Charge en este proveedor
// simulado. Se fija por instancia (no por petición): cada test construye
// su propio SimulatedProvider con el comportamiento que quiere reproducir.
type Behavior string

const (
	// BehaviorApprove aprueba la operación de inmediato.
	BehaviorApprove Behavior = "approve"
	// BehaviorReject rechaza la operación de inmediato.
	BehaviorReject Behavior = "reject"
	// BehaviorTimeout simula que el proveedor nunca llegó a procesar
	// nada: Charge falla con ErrProviderUnreachable, y GetStatus
	// tampoco tiene ninguna verdad guardada (PROCESSING indefinido).
	BehaviorTimeout Behavior = "timeout"
	// BehaviorApprovedButLost simula el escenario de riesgo central del
	// README: el proveedor SÍ aprueba y lo registra internamente, pero
	// Charge le devuelve a quien llama ErrProviderUnreachable, como si
	// la respuesta se hubiera perdido en el camino. GetStatus, después,
	// sí revela la verdad (APPROVED) al conciliar.
	BehaviorApprovedButLost Behavior = "approved_but_lost"
)

// SimulatedProvider implementa application.PaymentProvider. Guarda el
// resultado "real" de cada operación por separado de lo que Charge le
// devuelve a quien llama, para poder simular una respuesta perdida sin
// perder la capacidad de conciliar después.
type SimulatedProvider struct {
	mu       sync.Mutex
	behavior Behavior
	outcomes map[string]application.ProviderStatus
}

func NewSimulatedProvider(behavior Behavior) *SimulatedProvider {
	return &SimulatedProvider{
		behavior: behavior,
		outcomes: make(map[string]application.ProviderStatus),
	}
}

var _ application.PaymentProvider = (*SimulatedProvider)(nil)

func (p *SimulatedProvider) Charge(_ context.Context, req application.ChargeRequest) (application.ProviderStatus, error) {
	switch p.behavior {
	case BehaviorApprove:
		p.recordOutcome(req.ProviderReference, application.ProviderStatusApproved)
		return application.ProviderStatusApproved, nil

	case BehaviorReject:
		p.recordOutcome(req.ProviderReference, application.ProviderStatusRejected)
		return application.ProviderStatusRejected, nil

	case BehaviorApprovedButLost:
		// El proveedor sí aprueba y lo guarda "en su sistema" — pero la
		// respuesta hacia nosotros se simula perdida.
		p.recordOutcome(req.ProviderReference, application.ProviderStatusApproved)
		return "", application.ErrProviderUnreachable

	case BehaviorTimeout:
		fallthrough
	default:
		return "", application.ErrProviderUnreachable
	}
}

func (p *SimulatedProvider) GetStatus(_ context.Context, providerReference string) (application.ProviderStatus, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	status, ok := p.outcomes[providerReference]
	if !ok {
		// El proveedor tampoco tiene una respuesta definitiva todavía —
		// no es un error, es "vuelve a preguntar más tarde".
		return application.ProviderStatusProcessing, nil
	}
	return status, nil
}

// Resolve simula que el proveedor, con el tiempo, termina de procesar una
// operación que antes no tenía respuesta (BehaviorTimeout) — útil para
// probar que la conciliación funciona incluso después de una espera
// indefinida, sin tener que reconstruir el proveedor con otro Behavior.
func (p *SimulatedProvider) Resolve(providerReference string, status application.ProviderStatus) {
	p.recordOutcome(providerReference, status)
}

func (p *SimulatedProvider) recordOutcome(providerReference string, status application.ProviderStatus) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.outcomes[providerReference] = status
}

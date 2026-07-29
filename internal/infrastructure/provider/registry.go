package provider

import (
	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/application"
)

// Registry implementa application.ProviderRegistry con un simple mapa en
// memoria — quien arma la aplicación (cmd/api/main.go) registra ahí cada
// proveedor disponible una sola vez, al arrancar.
type Registry struct {
	providers map[string]application.PaymentProvider
}

func NewRegistry() *Registry {
	return &Registry{providers: make(map[string]application.PaymentProvider)}
}

var _ application.ProviderRegistry = (*Registry)(nil)

// Register agrega (o reemplaza) el proveedor asociado a name. Devuelve el
// propio Registry para poder encadenar varios Register seguidos al
// armar la aplicación.
func (r *Registry) Register(name string, provider application.PaymentProvider) *Registry {
	r.providers[name] = provider
	return r
}

func (r *Registry) Get(providerName string) (application.PaymentProvider, error) {
	provider, ok := r.providers[providerName]
	if !ok {
		return nil, application.ErrUnknownProvider
	}
	return provider, nil
}

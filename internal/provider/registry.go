package provider

import (
	"time"

	"github.com/tep/llm-cost-gateway/internal/domain"
	"github.com/tep/llm-cost-gateway/internal/netutil"
	contract "github.com/tep/llm-cost-gateway/internal/provider/contract"
	"github.com/tep/llm-cost-gateway/internal/provider/mock"
	"github.com/tep/llm-cost-gateway/internal/provider/openai_compatible"
)

type Registry struct {
	adapters map[string]contract.Adapter
}

type RegistryOption func(*registryConfig)

type registryConfig struct {
	publicOutboundOnly bool
}

func WithRegistryPublicOutboundOnly(enabled bool) RegistryOption {
	return func(cfg *registryConfig) {
		cfg.publicOutboundOnly = enabled
	}
}

func NewRegistry(opts ...RegistryOption) *Registry {
	cfg := registryConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}
	openAIOptions := []openai_compatible.Option{}
	if cfg.publicOutboundOnly {
		openAIOptions = append(openAIOptions, openai_compatible.WithHTTPClient(netutil.PublicOnlyHTTPClient(30*time.Second)))
	}
	return &Registry{
		adapters: map[string]contract.Adapter{
			TypeMock:             mock.NewAdapter(),
			TypeOpenAICompatible: openai_compatible.NewAdapter(openAIOptions...),
		},
	}
}

func (r *Registry) AdapterFor(providerType string) (contract.Adapter, error) {
	adapter, ok := r.adapters[providerType]
	if !ok {
		return nil, contract.NewError(domain.CodeProviderUnavailable, "provider adapter unavailable", nil)
	}
	return adapter, nil
}

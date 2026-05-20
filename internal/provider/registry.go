package provider

import (
	"github.com/tep/llm-cost-gateway/internal/domain"
	contract "github.com/tep/llm-cost-gateway/internal/provider/contract"
	"github.com/tep/llm-cost-gateway/internal/provider/mock"
	"github.com/tep/llm-cost-gateway/internal/provider/openai_compatible"
)

type Registry struct {
	adapters map[string]contract.Adapter
}

func NewRegistry() *Registry {
	return &Registry{
		adapters: map[string]contract.Adapter{
			TypeMock:             mock.NewAdapter(),
			TypeOpenAICompatible: openai_compatible.NewAdapter(),
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

package provider

import (
	"testing"

	"github.com/tep/llm-cost-gateway/internal/domain"
)

func TestRegistryReturnsAdapterForProviderType(t *testing.T) {
	registry := NewRegistry()

	tests := []struct {
		name         string
		providerType string
	}{
		{name: "mock", providerType: TypeMock},
		{name: "openai compatible", providerType: TypeOpenAICompatible},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter, err := registry.AdapterFor(tt.providerType)
			if err != nil {
				t.Fatalf("AdapterFor returned error: %v", err)
			}
			if adapter == nil {
				t.Fatal("AdapterFor returned nil adapter")
			}
		})
	}
}

func TestRegistryRejectsUnknownProviderType(t *testing.T) {
	registry := NewRegistry()

	_, err := registry.AdapterFor("unknown")
	if ErrorCode(err) != domain.CodeProviderUnavailable {
		t.Fatalf("AdapterFor error code = %q, want %q", ErrorCode(err), domain.CodeProviderUnavailable)
	}
}

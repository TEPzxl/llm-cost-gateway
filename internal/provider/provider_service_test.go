package provider

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/tep/llm-cost-gateway/internal/store"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
)

func TestServiceRejectsUnsafeProviderBaseURLWhenPublicOutboundOnly(t *testing.T) {
	service := NewService(nil, "0123456789abcdef0123456789abcdef", WithPublicOutboundOnly(true))
	_, err := service.CreateProvider(context.Background(), CreateProviderParams{
		OrgID:     uuid.New(),
		Name:      "unsafe-provider",
		Type:      TypeOpenAICompatible,
		BaseURL:   stringPtr("https://127.0.0.1/v1"),
		APIKey:    stringPtr("provider-secret-key"),
		TimeoutMS: 30000,
	})
	if err == nil {
		t.Fatal("CreateProvider returned nil error, want unsafe base_url rejection")
	}
}

func TestServiceCreateModelCreatesInitialPricingVersion(t *testing.T) {
	ctx := context.Background()
	st := openProviderServiceTestStore(t, ctx)
	orgID := createHealthTestOrg(t, ctx, st)
	service := NewService(st, "0123456789abcdef0123456789abcdef")

	provider, err := service.CreateProvider(ctx, CreateProviderParams{
		OrgID:     orgID,
		Name:      "pricing-provider",
		Type:      TypeMock,
		TimeoutMS: 30000,
	})
	if err != nil {
		t.Fatalf("CreateProvider returned error: %v", err)
	}
	model, err := service.CreateModel(ctx, CreateModelParams{
		OrgID:                          orgID,
		ProviderID:                     provider.ID,
		ProviderModelName:              "pricing-model",
		DisplayName:                    "Pricing Model",
		InputPriceMicroUSDPer1KTokens:  100,
		OutputPriceMicroUSDPer1KTokens: 200,
	})
	if err != nil {
		t.Fatalf("CreateModel returned error: %v", err)
	}

	version, err := st.Queries.GetActiveModelPricingVersion(ctx, db.GetActiveModelPricingVersionParams{
		OrgID:   orgID,
		ModelID: model.ID,
	})
	if err != nil {
		t.Fatalf("GetActiveModelPricingVersion returned error: %v", err)
	}
	if version.Version != 1 || version.Status != "active" {
		t.Fatalf("version = %d status = %q, want version 1 active", version.Version, version.Status)
	}
	if version.InputPriceMicroUsdPer1kTokens != 100 || version.OutputPriceMicroUsdPer1kTokens != 200 {
		t.Fatalf("version prices = %d/%d, want 100/200", version.InputPriceMicroUsdPer1kTokens, version.OutputPriceMicroUsdPer1kTokens)
	}
}

func openProviderServiceTestStore(t *testing.T, ctx context.Context) *store.Store {
	t.Helper()

	return newHealthTestStore(t, ctx)
}

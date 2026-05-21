package admin

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	secretcrypto "github.com/tep/llm-cost-gateway/internal/crypto"
	"github.com/tep/llm-cost-gateway/internal/provider"
	"github.com/tep/llm-cost-gateway/internal/store"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
	"github.com/tep/llm-cost-gateway/internal/testutil"
)

func TestSecretRotationServiceDryRunAndReencryptProviderSecrets(t *testing.T) {
	ctx := context.Background()
	st := openSecretRotationTestStore(t, ctx)
	orgID := createSecretRotationTestOrg(t, ctx, st)

	createService := provider.NewService(st, "0123456789abcdef0123456789abcdef")
	created, err := createService.CreateProvider(ctx, provider.CreateProviderParams{
		OrgID:     orgID,
		Name:      "rotate-provider",
		Type:      provider.TypeOpenAICompatible,
		BaseURL:   stringPtr("https://api.example.com/v1"),
		APIKey:    stringPtr("provider-secret-key"),
		TimeoutMS: 30000,
	})
	if err != nil {
		t.Fatalf("CreateProvider returned error: %v", err)
	}

	ring, err := secretcrypto.NewSecretKeyRing(2, map[int32]string{
		1: "0123456789abcdef0123456789abcdef",
		2: "fedcba98765432100123456789abcdef",
	})
	if err != nil {
		t.Fatalf("NewSecretKeyRing returned error: %v", err)
	}
	service := NewSecretRotationService(st, ring)

	dryRun, err := service.ReencryptProviderSecrets(ctx, ReencryptProviderSecretsParams{
		OrgID:  &orgID,
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("dry-run ReencryptProviderSecrets returned error: %v", err)
	}
	if dryRun.Scanned != 1 || dryRun.Rotated != 1 || dryRun.DryRun != true {
		t.Fatalf("dry-run result = %+v, want scanned=1 rotated=1 dry_run=true", dryRun)
	}
	secret := getProviderSecretForRotationTest(t, ctx, st, orgID, created.ID)
	if secret.KeyVersion != 1 {
		t.Fatalf("dry-run key version = %d, want unchanged 1", secret.KeyVersion)
	}

	result, err := service.ReencryptProviderSecrets(ctx, ReencryptProviderSecretsParams{
		OrgID:  &orgID,
		DryRun: false,
	})
	if err != nil {
		t.Fatalf("ReencryptProviderSecrets returned error: %v", err)
	}
	if result.Scanned != 1 || result.Rotated != 1 || result.DryRun {
		t.Fatalf("result = %+v, want scanned=1 rotated=1 dry_run=false", result)
	}
	secret = getProviderSecretForRotationTest(t, ctx, st, orgID, created.ID)
	if secret.KeyVersion != 2 {
		t.Fatalf("rotated key version = %d, want 2", secret.KeyVersion)
	}
	plaintext, err := ring.Open(secret.EncryptedApiKey, secret.Nonce, secret.KeyVersion)
	if err != nil {
		t.Fatalf("Open rotated secret returned error: %v", err)
	}
	if plaintext != "provider-secret-key" {
		t.Fatalf("rotated plaintext = %q, want provider-secret-key", plaintext)
	}
}

func openSecretRotationTestStore(t *testing.T, ctx context.Context) *store.Store {
	t.Helper()
	pool := testutil.OpenPostgres(t, ctx)
	st := store.New(pool)
	_, err := st.Pool.Exec(ctx, `
		TRUNCATE
			provider_secrets,
			providers,
			organizations
		CASCADE
	`)
	if err != nil {
		t.Fatalf("reset secret rotation test database: %v", err)
	}
	return st
}

func createSecretRotationTestOrg(t *testing.T, ctx context.Context, st *store.Store) uuid.UUID {
	t.Helper()
	now := time.Now().UTC()
	org, err := st.Queries.CreateOrganization(ctx, db.CreateOrganizationParams{
		ID:        uuid.New(),
		Name:      "Secret Rotation Org",
		Slug:      "secret-rotation-" + uuid.NewString()[:8],
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateOrganization returned error: %v", err)
	}
	return org.ID
}

func getProviderSecretForRotationTest(t *testing.T, ctx context.Context, st *store.Store, orgID uuid.UUID, providerID uuid.UUID) db.ProviderSecret {
	t.Helper()
	secret, err := st.Queries.GetProviderSecret(ctx, db.GetProviderSecretParams{
		OrgID:      orgID,
		ProviderID: providerID,
	})
	if err != nil {
		t.Fatalf("GetProviderSecret returned error: %v", err)
	}
	return secret
}

func stringPtr(value string) *string {
	return &value
}

package admin

import (
	"context"
	"time"

	"github.com/google/uuid"
	secretcrypto "github.com/tep/llm-cost-gateway/internal/crypto"
	"github.com/tep/llm-cost-gateway/internal/store"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
)

type SecretRotationService struct {
	store   *store.Store
	keyRing *secretcrypto.SecretKeyRing
	now     func() time.Time
}

func NewSecretRotationService(st *store.Store, keyRing *secretcrypto.SecretKeyRing) *SecretRotationService {
	return &SecretRotationService{
		store:   st,
		keyRing: keyRing,
		now:     func() time.Time { return time.Now().UTC() },
	}
}

type ReencryptProviderSecretsParams struct {
	OrgID  *uuid.UUID
	DryRun bool
}

type ReencryptProviderSecretsResult struct {
	DryRun  bool
	Scanned int
	Rotated int
	Skipped int
}

func (s *SecretRotationService) ReencryptProviderSecrets(ctx context.Context, params ReencryptProviderSecretsParams) (ReencryptProviderSecretsResult, error) {
	result := ReencryptProviderSecretsResult{DryRun: params.DryRun}
	secrets, err := s.store.Queries.ListProviderSecretsForRotation(ctx, params.OrgID)
	if err != nil {
		return result, err
	}
	result.Scanned = len(secrets)
	for _, secret := range secrets {
		if secret.KeyVersion == s.keyRing.ActiveVersion() {
			result.Skipped++
			continue
		}
		plaintext, err := s.keyRing.Open(secret.EncryptedApiKey, secret.Nonce, secret.KeyVersion)
		if err != nil {
			return result, err
		}
		if params.DryRun {
			result.Rotated++
			continue
		}
		sealed, err := s.keyRing.Seal(plaintext)
		if err != nil {
			return result, err
		}
		if _, err := s.store.Queries.UpdateProviderSecretCiphertext(ctx, db.UpdateProviderSecretCiphertextParams{
			OrgID:           secret.OrgID,
			ProviderID:      secret.ProviderID,
			ID:              secret.ID,
			EncryptedApiKey: sealed.Encrypted,
			Nonce:           sealed.Nonce,
			KeyVersion:      sealed.KeyVersion,
			UpdatedAt:       s.now(),
		}); err != nil {
			return result, err
		}
		result.Rotated++
	}
	return result, nil
}

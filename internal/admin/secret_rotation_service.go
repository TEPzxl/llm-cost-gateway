package admin

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/tep/llm-cost-gateway/internal/budget"
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

type ReencryptWebhookSecretsParams struct {
	OrgID  *uuid.UUID
	DryRun bool
}

type ReencryptWebhookSecretsResult struct {
	DryRun  bool
	Scanned int
	Rotated int
	Skipped int
}

func reencryptWebhookSecretValue(keyRing *secretcrypto.SecretKeyRing, stored pgtype.Text) (pgtype.Text, bool, error) {
	if !stored.Valid || strings.TrimSpace(stored.String) == "" {
		return stored, false, nil
	}
	version, encrypted, err := budget.WebhookSecretKeyVersion(stored)
	if err != nil {
		return pgtype.Text{}, false, err
	}
	if encrypted && version == keyRing.ActiveVersion() {
		return stored, false, nil
	}
	plaintext, err := budget.OpenWebhookSecret(keyRing, stored)
	if err != nil {
		return pgtype.Text{}, false, err
	}
	rotated, err := budget.SealWebhookSecret(keyRing, plaintext)
	if err != nil {
		return pgtype.Text{}, false, err
	}
	return rotated, true, nil
}

func (s *SecretRotationService) ReencryptWebhookSecrets(ctx context.Context, params ReencryptWebhookSecretsParams) (ReencryptWebhookSecretsResult, error) {
	result := ReencryptWebhookSecretsResult{DryRun: params.DryRun}
	rows, err := s.store.Pool.Query(ctx, `
		SELECT id, org_id, webhook_secret
		FROM budget_alerts
		WHERE webhook_secret IS NOT NULL
		  AND ($1::uuid IS NULL OR org_id = $1::uuid)
		ORDER BY org_id, id
	`, params.OrgID)
	if err != nil {
		return result, err
	}
	defer rows.Close()

	for rows.Next() {
		var alertID uuid.UUID
		var orgID uuid.UUID
		var webhookSecret pgtype.Text
		if err := rows.Scan(&alertID, &orgID, &webhookSecret); err != nil {
			return result, err
		}
		result.Scanned++
		rotated, changed, err := reencryptWebhookSecretValue(s.keyRing, webhookSecret)
		if err != nil {
			return result, err
		}
		if !changed {
			result.Skipped++
			continue
		}
		if params.DryRun {
			result.Rotated++
			continue
		}
		if _, err := s.store.Pool.Exec(ctx, `
			UPDATE budget_alerts
			SET webhook_secret = $3,
			    updated_at = $4
			WHERE org_id = $1
			  AND id = $2
		`, orgID, alertID, rotated, s.now()); err != nil {
			return result, err
		}
		result.Rotated++
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	return result, nil
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

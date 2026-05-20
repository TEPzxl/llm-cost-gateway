package provider

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	secretcrypto "github.com/tep/llm-cost-gateway/internal/crypto"
	"github.com/tep/llm-cost-gateway/internal/domain"
	"github.com/tep/llm-cost-gateway/internal/store"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
)

const (
	HealthStatusHealthy   = "healthy"
	HealthStatusUnhealthy = "unhealthy"
)

type HealthService struct {
	store               *store.Store
	secretEncryptionKey string
	client              *http.Client
	clock               func() time.Time
}

func NewHealthService(st *store.Store, secretEncryptionKey string) *HealthService {
	return &HealthService{
		store:               st,
		secretEncryptionKey: secretEncryptionKey,
		client:              http.DefaultClient,
		clock:               func() time.Time { return time.Now().UTC() },
	}
}

func (s *HealthService) List(ctx context.Context, orgID uuid.UUID) ([]db.Provider, error) {
	return s.store.Queries.ListProviders(ctx, orgID)
}

func (s *HealthService) Check(ctx context.Context, orgID uuid.UUID, providerID uuid.UUID) (db.Provider, error) {
	item, err := s.store.Queries.GetProvider(ctx, db.GetProviderParams{
		OrgID: orgID,
		ID:    providerID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.Provider{}, ErrProviderNotFound
		}
		return db.Provider{}, err
	}

	status := HealthStatusHealthy
	var errorCode string
	var errorMessage string
	if item.Type == TypeOpenAICompatible {
		errorCode, errorMessage = s.checkOpenAICompatible(ctx, item)
		if errorCode != "" {
			status = HealthStatusUnhealthy
		}
	}

	now := s.clock()
	return s.store.Queries.UpdateProviderHealth(ctx, db.UpdateProviderHealthParams{
		OrgID:               orgID,
		ID:                  providerID,
		LastHealthStatus:    textValue(status),
		LastHealthCheckedAt: &now,
		LastErrorCode:       nullableText(errorCode),
		LastErrorMessage:    nullableText(errorMessage),
		UpdatedAt:           now,
	})
}

func (s *HealthService) checkOpenAICompatible(ctx context.Context, item db.Provider) (string, string) {
	if !item.BaseUrl.Valid || strings.TrimSpace(item.BaseUrl.String) == "" {
		return domain.CodeProviderError, "provider base_url is required"
	}
	secret, err := s.store.Queries.GetProviderSecret(ctx, db.GetProviderSecretParams{
		OrgID:      item.OrgID,
		ProviderID: item.ID,
	})
	if err != nil {
		return domain.CodeProviderError, "provider api key is required"
	}
	box, err := secretcrypto.NewSecretBox(s.secretEncryptionKey)
	if err != nil {
		return domain.CodeProviderError, "provider secret configuration is invalid"
	}
	apiKey, err := box.Open(secret.EncryptedApiKey, secret.Nonce)
	if err != nil {
		return domain.CodeProviderError, "provider api key decrypt failed"
	}

	timeoutMS := item.TimeoutMs
	if timeoutMS <= 0 {
		timeoutMS = 30000
	}
	callCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMS)*time.Millisecond)
	defer cancel()

	req, err := http.NewRequestWithContext(callCtx, http.MethodGet, providerModelsURL(item.BaseUrl.String), nil)
	if err != nil {
		return domain.CodeProviderError, "build provider health request failed"
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		if isProviderHealthTimeout(callCtx, err) {
			return domain.CodeProviderTimeout, "provider health check timed out"
		}
		return domain.CodeProviderUnavailable, "provider health check failed"
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		code := domain.CodeProviderError
		if resp.StatusCode >= 500 {
			code = domain.CodeProviderUnavailable
		}
		return code, fmt.Sprintf("provider health check returned status %d", resp.StatusCode)
	}
	return "", ""
}

func providerModelsURL(baseURL string) string {
	return strings.TrimRight(baseURL, "/") + "/models"
}

func isProviderHealthTimeout(ctx context.Context, err error) bool {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func textValue(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: true}
}

func nullableText(value string) pgtype.Text {
	if strings.TrimSpace(value) == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}

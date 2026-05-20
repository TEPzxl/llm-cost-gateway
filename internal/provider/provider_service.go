package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	secretcrypto "github.com/tep/llm-cost-gateway/internal/crypto"
	"github.com/tep/llm-cost-gateway/internal/store"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
)

const (
	TypeMock             = "mock"
	TypeOpenAICompatible = "openai_compatible"
	defaultKeyVersion    = int32(1)
)

var ErrProviderNotFound = errors.New("provider not found")

type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}

type Service struct {
	store               *store.Store
	secretEncryptionKey string
}

type CreateProviderParams struct {
	OrgID     uuid.UUID
	Name      string
	Type      string
	BaseURL   *string
	APIKey    *string
	TimeoutMS int32
}

type CreateModelParams struct {
	OrgID                          uuid.UUID
	ProviderID                     uuid.UUID
	ProviderModelName              string
	DisplayName                    string
	InputPriceMicroUSDPer1KTokens  int64
	OutputPriceMicroUSDPer1KTokens int64
	ContextWindow                  *int32
}

func NewService(st *store.Store, secretEncryptionKey string) *Service {
	return &Service{
		store:               st,
		secretEncryptionKey: secretEncryptionKey,
	}
}

func (s *Service) CreateProvider(ctx context.Context, params CreateProviderParams) (db.Provider, error) {
	if err := validateProviderParams(params); err != nil {
		return db.Provider{}, err
	}

	var created db.Provider
	now := time.Now().UTC()
	err := s.store.ExecTx(ctx, func(q *db.Queries) error {
		provider, err := q.CreateProvider(ctx, db.CreateProviderParams{
			ID:        uuid.New(),
			OrgID:     params.OrgID,
			Name:      strings.TrimSpace(params.Name),
			Type:      params.Type,
			BaseUrl:   newPgText(params.BaseURL),
			TimeoutMs: params.TimeoutMS,
			Status:    "active",
			CreatedAt: now,
			UpdatedAt: now,
		})
		if err != nil {
			return err
		}
		created = provider

		if params.Type != TypeOpenAICompatible {
			return nil
		}

		box, err := secretcrypto.NewSecretBox(s.secretEncryptionKey)
		if err != nil {
			return err
		}
		sealed, err := box.Seal(strings.TrimSpace(*params.APIKey))
		if err != nil {
			return err
		}
		_, err = q.UpsertProviderSecret(ctx, db.UpsertProviderSecretParams{
			ID:              uuid.New(),
			OrgID:           params.OrgID,
			ProviderID:      provider.ID,
			EncryptedApiKey: sealed.Encrypted,
			Nonce:           sealed.Nonce,
			KeyVersion:      defaultKeyVersion,
			CreatedAt:       now,
			UpdatedAt:       now,
		})
		return err
	})
	if err != nil {
		return db.Provider{}, err
	}

	return created, nil
}

func (s *Service) ListProviders(ctx context.Context, orgID uuid.UUID) ([]db.Provider, error) {
	return s.store.Queries.ListProviders(ctx, orgID)
}

func (s *Service) CreateModel(ctx context.Context, params CreateModelParams) (db.Model, error) {
	if err := validateModelParams(params); err != nil {
		return db.Model{}, err
	}

	if _, err := s.store.Queries.GetProvider(ctx, db.GetProviderParams{
		OrgID: params.OrgID,
		ID:    params.ProviderID,
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.Model{}, ErrProviderNotFound
		}
		return db.Model{}, err
	}

	now := time.Now().UTC()
	return s.store.Queries.CreateModel(ctx, db.CreateModelParams{
		ID:                             uuid.New(),
		OrgID:                          params.OrgID,
		ProviderID:                     params.ProviderID,
		ProviderModelName:              strings.TrimSpace(params.ProviderModelName),
		DisplayName:                    strings.TrimSpace(params.DisplayName),
		InputPriceMicroUsdPer1kTokens:  params.InputPriceMicroUSDPer1KTokens,
		OutputPriceMicroUsdPer1kTokens: params.OutputPriceMicroUSDPer1KTokens,
		ContextWindow:                  newPgInt4(params.ContextWindow),
		Status:                         "active",
		CreatedAt:                      now,
		UpdatedAt:                      now,
	})
}

func (s *Service) ListModels(ctx context.Context, orgID uuid.UUID) ([]db.Model, error) {
	return s.store.Queries.ListModels(ctx, orgID)
}

func validateProviderParams(params CreateProviderParams) error {
	if params.OrgID == uuid.Nil {
		return validationError("org_id is required")
	}
	if strings.TrimSpace(params.Name) == "" {
		return validationError("name is required")
	}
	if params.TimeoutMS <= 0 {
		return validationError("timeout_ms must be greater than zero")
	}

	switch params.Type {
	case TypeMock:
		return nil
	case TypeOpenAICompatible:
		if params.BaseURL == nil || strings.TrimSpace(*params.BaseURL) == "" {
			return validationError("base_url is required")
		}
		if params.APIKey == nil || strings.TrimSpace(*params.APIKey) == "" {
			return validationError("api_key is required")
		}
		return nil
	default:
		return validationError("provider type must be mock or openai_compatible")
	}
}

func validateModelParams(params CreateModelParams) error {
	if params.OrgID == uuid.Nil {
		return validationError("org_id is required")
	}
	if params.ProviderID == uuid.Nil {
		return validationError("provider_id is required")
	}
	if strings.TrimSpace(params.ProviderModelName) == "" {
		return validationError("provider_model_name is required")
	}
	if strings.TrimSpace(params.DisplayName) == "" {
		return validationError("display_name is required")
	}
	if params.InputPriceMicroUSDPer1KTokens < 0 || params.OutputPriceMicroUSDPer1KTokens < 0 {
		return validationError("model prices must be non-negative")
	}
	if params.ContextWindow != nil && *params.ContextWindow <= 0 {
		return validationError("context_window must be greater than zero")
	}
	return nil
}

func validationError(message string) *ValidationError {
	return &ValidationError{Message: message}
}

func newPgText(value *string) pgtype.Text {
	if value == nil || strings.TrimSpace(*value) == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: strings.TrimSpace(*value), Valid: true}
}

func newPgInt4(value *int32) pgtype.Int4 {
	if value == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: *value, Valid: true}
}

func IsValidationError(err error) (*ValidationError, bool) {
	var validation *ValidationError
	if errors.As(err, &validation) {
		return validation, true
	}
	return nil, false
}

func IsUniqueViolation(err error) bool {
	type sqlState interface {
		SQLState() string
	}
	var state sqlState
	if errors.As(err, &state) {
		return state.SQLState() == "23505"
	}
	return false
}

func WrapUnexpected(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("provider service: %w", err)
}

package costing

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/tep/llm-cost-gateway/internal/store"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
)

const (
	PricingStatusActive     = "active"
	PricingStatusSuperseded = "superseded"
)

var ErrModelPricingNotFound = errors.New("model pricing not found")

type PricingService struct {
	store *store.Store
	clock func() time.Time
}

type PricingOption func(*PricingService)

func WithPricingClock(clock func() time.Time) PricingOption {
	return func(s *PricingService) {
		if clock != nil {
			s.clock = clock
		}
	}
}

func NewPricingService(st *store.Store, opts ...PricingOption) *PricingService {
	service := &PricingService{
		store: st,
		clock: func() time.Time { return time.Now().UTC() },
	}
	for _, opt := range opts {
		opt(service)
	}
	return service
}

type UpdateModelPricingParams struct {
	OrgID                          uuid.UUID
	ModelID                        uuid.UUID
	InputPriceMicroUSDPer1KTokens  int64
	OutputPriceMicroUSDPer1KTokens int64
}

type UpdateModelPricingResult struct {
	Model           db.Model
	PreviousVersion db.ModelPricingVersion
	PricingVersion  db.ModelPricingVersion
}

func (s *PricingService) CreateInitialVersion(ctx context.Context, model db.Model, effectiveFrom time.Time) (db.ModelPricingVersion, error) {
	return s.CreateInitialVersionWithQueries(ctx, s.store.Queries, model, effectiveFrom)
}

func (s *PricingService) CreateInitialVersionWithQueries(ctx context.Context, q *db.Queries, model db.Model, effectiveFrom time.Time) (db.ModelPricingVersion, error) {
	if model.OrgID == uuid.Nil || model.ID == uuid.Nil {
		return db.ModelPricingVersion{}, fmt.Errorf("%w: model identity is required", ErrInvalidCostInput)
	}
	if effectiveFrom.IsZero() {
		effectiveFrom = s.clock()
	}
	return q.CreateModelPricingVersion(ctx, db.CreateModelPricingVersionParams{
		ID:                             uuid.New(),
		OrgID:                          model.OrgID,
		ModelID:                        model.ID,
		Version:                        1,
		InputPriceMicroUsdPer1kTokens:  model.InputPriceMicroUsdPer1kTokens,
		OutputPriceMicroUsdPer1kTokens: model.OutputPriceMicroUsdPer1kTokens,
		Status:                         PricingStatusActive,
		EffectiveFrom:                  effectiveFrom,
		CreatedAt:                      effectiveFrom,
	})
}

func (s *PricingService) UpdateModelPricing(ctx context.Context, params UpdateModelPricingParams) (UpdateModelPricingResult, error) {
	if params.OrgID == uuid.Nil {
		return UpdateModelPricingResult{}, fmt.Errorf("%w: org_id is required", ErrInvalidCostInput)
	}
	if params.ModelID == uuid.Nil {
		return UpdateModelPricingResult{}, fmt.Errorf("%w: model_id is required", ErrInvalidCostInput)
	}
	if params.InputPriceMicroUSDPer1KTokens < 0 || params.OutputPriceMicroUSDPer1KTokens < 0 {
		return UpdateModelPricingResult{}, fmt.Errorf("%w: model prices must be non-negative", ErrInvalidCostInput)
	}

	var result UpdateModelPricingResult
	now := s.clock()
	err := s.store.ExecTx(ctx, func(q *db.Queries) error {
		model, err := q.GetModel(ctx, db.GetModelParams{OrgID: params.OrgID, ID: params.ModelID})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrModelPricingNotFound
			}
			return err
		}

		nextVersion, err := q.GetNextModelPricingVersionNumber(ctx, db.GetNextModelPricingVersionNumberParams{
			OrgID:   params.OrgID,
			ModelID: params.ModelID,
		})
		if err != nil {
			return err
		}
		previous, err := q.SupersedeActiveModelPricingVersion(ctx, db.SupersedeActiveModelPricingVersionParams{
			OrgID:       params.OrgID,
			ModelID:     params.ModelID,
			EffectiveTo: &now,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrModelPricingNotFound
			}
			return err
		}
		updatedModel, err := q.UpdateModelPricing(ctx, db.UpdateModelPricingParams{
			OrgID:                          params.OrgID,
			ID:                             params.ModelID,
			InputPriceMicroUsdPer1kTokens:  params.InputPriceMicroUSDPer1KTokens,
			OutputPriceMicroUsdPer1kTokens: params.OutputPriceMicroUSDPer1KTokens,
			UpdatedAt:                      now,
		})
		if err != nil {
			return err
		}
		version, err := q.CreateModelPricingVersion(ctx, db.CreateModelPricingVersionParams{
			ID:                             uuid.New(),
			OrgID:                          params.OrgID,
			ModelID:                        model.ID,
			Version:                        nextVersion,
			InputPriceMicroUsdPer1kTokens:  params.InputPriceMicroUSDPer1KTokens,
			OutputPriceMicroUsdPer1kTokens: params.OutputPriceMicroUSDPer1KTokens,
			Status:                         PricingStatusActive,
			EffectiveFrom:                  now,
			CreatedAt:                      now,
		})
		if err != nil {
			return err
		}

		result = UpdateModelPricingResult{
			Model:           updatedModel,
			PreviousVersion: previous,
			PricingVersion:  version,
		}
		return nil
	})
	if err != nil {
		return UpdateModelPricingResult{}, err
	}
	return result, nil
}

func (s *PricingService) ListModelPricingVersions(ctx context.Context, orgID uuid.UUID, modelID uuid.UUID) ([]db.ModelPricingVersion, error) {
	if _, err := s.store.Queries.GetModel(ctx, db.GetModelParams{OrgID: orgID, ID: modelID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrModelPricingNotFound
		}
		return nil, err
	}
	return s.store.Queries.ListModelPricingVersions(ctx, db.ListModelPricingVersionsParams{
		OrgID:   orgID,
		ModelID: modelID,
	})
}

func (s *PricingService) GetActiveModelPricingVersion(ctx context.Context, orgID uuid.UUID, modelID uuid.UUID) (db.ModelPricingVersion, error) {
	version, err := s.store.Queries.GetActiveModelPricingVersion(ctx, db.GetActiveModelPricingVersionParams{
		OrgID:   orgID,
		ModelID: modelID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.ModelPricingVersion{}, ErrModelPricingNotFound
		}
		return db.ModelPricingVersion{}, err
	}
	return version, nil
}

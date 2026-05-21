package events

import (
	"context"
	"time"

	"github.com/google/uuid"
)

const (
	EventUsageRecorded  = "llm.usage.recorded"
	EventRequestBlocked = "llm.request.blocked"
)

type UsageEvent struct {
	Type              string     `json:"type"`
	OrgID             uuid.UUID  `json:"org_id"`
	APIKeyID          *uuid.UUID `json:"api_key_id,omitempty"`
	ProviderID        *uuid.UUID `json:"provider_id,omitempty"`
	ModelID           *uuid.UUID `json:"model_id,omitempty"`
	RequestID         uuid.UUID  `json:"request_id"`
	PromptTokens      int64      `json:"prompt_tokens"`
	CompletionTokens  int64      `json:"completion_tokens"`
	TotalTokens       int64      `json:"total_tokens"`
	TotalCostMicroUSD int64      `json:"total_cost_micro_usd"`
	Status            string     `json:"status"`
	CreatedAt         time.Time  `json:"created_at"`
}

type Publisher interface {
	Publish(ctx context.Context, event UsageEvent) error
}

type DisabledPublisher struct{}

func (DisabledPublisher) Publish(context.Context, UsageEvent) error {
	return nil
}

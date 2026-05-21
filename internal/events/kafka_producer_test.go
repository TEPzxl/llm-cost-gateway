package events

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestDisabledPublisherDoesNotSend(t *testing.T) {
	publisher := DisabledPublisher{}
	if err := publisher.Publish(context.Background(), UsageEvent{Type: EventUsageRecorded}); err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}
}

func TestUsageEventJSONDoesNotContainSensitiveContent(t *testing.T) {
	event := UsageEvent{
		Type:              EventUsageRecorded,
		OrgID:             uuid.New(),
		APIKeyID:          uuidPtr(uuid.New()),
		ProviderID:        uuidPtr(uuid.New()),
		ModelID:           uuidPtr(uuid.New()),
		RequestID:         uuid.New(),
		PromptTokens:      20,
		CompletionTokens:  30,
		TotalTokens:       50,
		TotalCostMicroUSD: 8,
		Status:            "success",
		CreatedAt:         time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC),
	}

	body, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal usage event: %v", err)
	}
	payload := string(body)
	if !strings.Contains(payload, EventUsageRecorded) {
		t.Fatalf("payload = %s, want event type", payload)
	}
	for _, secret := range []string{"hello prompt", "mock response", "llmgw_live_", "authorization"} {
		if strings.Contains(strings.ToLower(payload), strings.ToLower(secret)) {
			t.Fatalf("payload contains sensitive value %q: %s", secret, payload)
		}
	}
}

func uuidPtr(value uuid.UUID) *uuid.UUID {
	return &value
}

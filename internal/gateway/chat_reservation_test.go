package gateway

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	contract "github.com/tep/llm-cost-gateway/internal/provider/contract"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
)

func TestEstimateReservationCostUsesContextWindowWhenMaxTokensMissing(t *testing.T) {
	request := ChatCompletionRequest{
		Model: "fast-chat",
		Messages: []contract.ChatMessage{
			{Role: "user", Content: "12345678"},
		},
	}
	cost, err := estimateReservationCost(request, []db.Model{{
		ID:                             uuid.New(),
		InputPriceMicroUsdPer1kTokens:  100,
		OutputPriceMicroUsdPer1kTokens: 200,
		ContextWindow:                  pgtype.Int4{Int32: 10, Valid: true},
	}})
	if err != nil {
		t.Fatalf("estimateReservationCost returned error: %v", err)
	}

	// 8 runes ~= 2 prompt tokens, so the conservative output reservation is 8 tokens.
	// Cost truncates micro USD per 1k tokens: prompt 2*100/1000 + output 8*200/1000 = 1.
	if cost != 1 {
		t.Fatalf("cost = %d, want 1", cost)
	}
}

func TestEstimateReservationCostUsesDefaultCompletionTokensWhenContextWindowMissing(t *testing.T) {
	request := ChatCompletionRequest{
		Model: "fast-chat",
		Messages: []contract.ChatMessage{
			{Role: "user", Content: "12345678"},
		},
	}
	cost, err := estimateReservationCost(request, []db.Model{{
		ID:                             uuid.New(),
		InputPriceMicroUsdPer1kTokens:  1000,
		OutputPriceMicroUsdPer1kTokens: 1000,
	}})
	if err != nil {
		t.Fatalf("estimateReservationCost returned error: %v", err)
	}

	want := int64(2 + defaultReservationCompletionTokens)
	if cost != want {
		t.Fatalf("cost = %d, want %d", cost, want)
	}
}

func TestEstimateReservationCostUsesMaxTokensWhenProvided(t *testing.T) {
	maxTokens := 5
	request := ChatCompletionRequest{
		Model:     "fast-chat",
		MaxTokens: &maxTokens,
		Messages: []contract.ChatMessage{
			{Role: "user", Content: "12345678"},
		},
	}
	cost, err := estimateReservationCost(request, []db.Model{{
		ID:                             uuid.New(),
		InputPriceMicroUsdPer1kTokens:  100,
		OutputPriceMicroUsdPer1kTokens: 200,
		ContextWindow:                  pgtype.Int4{Int32: 10, Valid: true},
	}})
	if err != nil {
		t.Fatalf("estimateReservationCost returned error: %v", err)
	}

	// Prompt 2*100/1000 + output 5*200/1000 = 1.
	if cost != 1 {
		t.Fatalf("cost = %d, want 1", cost)
	}
}

package costing

import (
	"errors"
	"math"
	"testing"
)

func TestCalculatorCalculate(t *testing.T) {
	calculator := NewCalculator()

	tests := []struct {
		name                string
		input               CalculateInput
		wantInputCostMicro  int64
		wantOutputCostMicro int64
		wantTotalCostMicro  int64
	}{
		{
			name: "normal input and output tokens",
			input: CalculateInput{
				PromptTokens:                   20,
				CompletionTokens:               30,
				InputPriceMicroUSDPer1KTokens:  100,
				OutputPriceMicroUSDPer1KTokens: 200,
			},
			wantInputCostMicro:  2,
			wantOutputCostMicro: 6,
			wantTotalCostMicro:  8,
		},
		{
			name: "zero tokens",
			input: CalculateInput{
				PromptTokens:                   0,
				CompletionTokens:               0,
				InputPriceMicroUSDPer1KTokens:  100,
				OutputPriceMicroUSDPer1KTokens: 200,
			},
			wantInputCostMicro:  0,
			wantOutputCostMicro: 0,
			wantTotalCostMicro:  0,
		},
		{
			name: "large token counts",
			input: CalculateInput{
				PromptTokens:                   2_000_000,
				CompletionTokens:               3_000_000,
				InputPriceMicroUSDPer1KTokens:  1_234_567,
				OutputPriceMicroUSDPer1KTokens: 2_345_678,
			},
			wantInputCostMicro:  2_469_134_000,
			wantOutputCostMicro: 7_037_034_000,
			wantTotalCostMicro:  9_506_168_000,
		},
		{
			name: "integer division truncates fractional micro usd",
			input: CalculateInput{
				PromptTokens:                   1,
				CompletionTokens:               1,
				InputPriceMicroUSDPer1KTokens:  999,
				OutputPriceMicroUSDPer1KTokens: 1001,
			},
			wantInputCostMicro:  0,
			wantOutputCostMicro: 1,
			wantTotalCostMicro:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := calculator.Calculate(tt.input)
			if err != nil {
				t.Fatalf("Calculate returned error: %v", err)
			}
			if got.InputCostMicro != tt.wantInputCostMicro {
				t.Fatalf("input cost = %d, want %d", got.InputCostMicro, tt.wantInputCostMicro)
			}
			if got.OutputCostMicro != tt.wantOutputCostMicro {
				t.Fatalf("output cost = %d, want %d", got.OutputCostMicro, tt.wantOutputCostMicro)
			}
			if got.TotalCostMicro != tt.wantTotalCostMicro {
				t.Fatalf("total cost = %d, want %d", got.TotalCostMicro, tt.wantTotalCostMicro)
			}
			if got.PricingSnapshot.InputPriceMicroUSDPer1KTokens != tt.input.InputPriceMicroUSDPer1KTokens {
				t.Fatalf("snapshot input price = %d, want %d", got.PricingSnapshot.InputPriceMicroUSDPer1KTokens, tt.input.InputPriceMicroUSDPer1KTokens)
			}
			if got.PricingSnapshot.OutputPriceMicroUSDPer1KTokens != tt.input.OutputPriceMicroUSDPer1KTokens {
				t.Fatalf("snapshot output price = %d, want %d", got.PricingSnapshot.OutputPriceMicroUSDPer1KTokens, tt.input.OutputPriceMicroUSDPer1KTokens)
			}
			if got.PricingSnapshot.RoundingMode != RoundingModeTruncateToMicroUSD {
				t.Fatalf("snapshot rounding mode = %q, want %q", got.PricingSnapshot.RoundingMode, RoundingModeTruncateToMicroUSD)
			}
		})
	}
}

func TestCalculatorDetectsInt64Overflow(t *testing.T) {
	calculator := NewCalculator()

	_, err := calculator.Calculate(CalculateInput{
		PromptTokens:                   math.MaxInt64,
		InputPriceMicroUSDPer1KTokens:  math.MaxInt64,
		OutputPriceMicroUSDPer1KTokens: 1,
	})
	if !errors.Is(err, ErrCostOverflow) {
		t.Fatalf("Calculate error = %v, want ErrCostOverflow", err)
	}
}

func TestCalculatorRejectsNegativeInputs(t *testing.T) {
	calculator := NewCalculator()

	tests := []struct {
		name  string
		input CalculateInput
	}{
		{
			name: "negative prompt tokens",
			input: CalculateInput{
				PromptTokens:                   -1,
				InputPriceMicroUSDPer1KTokens:  100,
				OutputPriceMicroUSDPer1KTokens: 200,
			},
		},
		{
			name: "negative completion tokens",
			input: CalculateInput{
				CompletionTokens:               -1,
				InputPriceMicroUSDPer1KTokens:  100,
				OutputPriceMicroUSDPer1KTokens: 200,
			},
		},
		{
			name: "negative input price",
			input: CalculateInput{
				PromptTokens:                   1,
				CompletionTokens:               1,
				InputPriceMicroUSDPer1KTokens:  -1,
				OutputPriceMicroUSDPer1KTokens: 200,
			},
		},
		{
			name: "negative output price",
			input: CalculateInput{
				PromptTokens:                   1,
				CompletionTokens:               1,
				InputPriceMicroUSDPer1KTokens:  100,
				OutputPriceMicroUSDPer1KTokens: -1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := calculator.Calculate(tt.input)
			if !errors.Is(err, ErrInvalidCostInput) {
				t.Fatalf("Calculate error = %v, want ErrInvalidCostInput", err)
			}
		})
	}
}

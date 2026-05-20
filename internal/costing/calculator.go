package costing

import (
	"errors"
	"fmt"
	"math"
	"math/big"

	"github.com/tep/llm-cost-gateway/internal/domain"
)

const currencyUSD = "USD"
const RoundingModeTruncateToMicroUSD = "truncate_to_micro_usd"

var ErrInvalidCostInput = errors.New("invalid cost input")
var ErrCostOverflow = errors.New("cost overflow")

type Calculator struct{}

type CalculateInput struct {
	PromptTokens                   int64
	CompletionTokens               int64
	InputPriceMicroUSDPer1KTokens  int64
	OutputPriceMicroUSDPer1KTokens int64
}

func NewCalculator() *Calculator {
	return &Calculator{}
}

func (c *Calculator) Calculate(input CalculateInput) (domain.CostCalculation, error) {
	if input.PromptTokens < 0 {
		return domain.CostCalculation{}, fmt.Errorf("%w: prompt tokens must be non-negative", ErrInvalidCostInput)
	}
	if input.CompletionTokens < 0 {
		return domain.CostCalculation{}, fmt.Errorf("%w: completion tokens must be non-negative", ErrInvalidCostInput)
	}
	if input.InputPriceMicroUSDPer1KTokens < 0 {
		return domain.CostCalculation{}, fmt.Errorf("%w: input price must be non-negative", ErrInvalidCostInput)
	}
	if input.OutputPriceMicroUSDPer1KTokens < 0 {
		return domain.CostCalculation{}, fmt.Errorf("%w: output price must be non-negative", ErrInvalidCostInput)
	}

	inputCostMicro, err := costMicro(input.PromptTokens, input.InputPriceMicroUSDPer1KTokens)
	if err != nil {
		return domain.CostCalculation{}, err
	}
	outputCostMicro, err := costMicro(input.CompletionTokens, input.OutputPriceMicroUSDPer1KTokens)
	if err != nil {
		return domain.CostCalculation{}, err
	}
	if inputCostMicro > math.MaxInt64-outputCostMicro {
		return domain.CostCalculation{}, fmt.Errorf("%w: total cost exceeds int64", ErrCostOverflow)
	}

	return domain.CostCalculation{
		Currency:        currencyUSD,
		InputCostMicro:  inputCostMicro,
		OutputCostMicro: outputCostMicro,
		TotalCostMicro:  inputCostMicro + outputCostMicro,
		PricingSnapshot: domain.PricingSnapshot{
			InputPriceMicroUSDPer1KTokens:  input.InputPriceMicroUSDPer1KTokens,
			OutputPriceMicroUSDPer1KTokens: input.OutputPriceMicroUSDPer1KTokens,
			RoundingMode:                   RoundingModeTruncateToMicroUSD,
		},
	}, nil
}

func costMicro(tokens int64, priceMicroUSDPer1KTokens int64) (int64, error) {
	product := new(big.Int).Mul(big.NewInt(tokens), big.NewInt(priceMicroUSDPer1KTokens))
	value := new(big.Int).Quo(product, big.NewInt(1000))
	if !value.IsInt64() {
		return 0, fmt.Errorf("%w: cost exceeds int64", ErrCostOverflow)
	}
	return value.Int64(), nil
}

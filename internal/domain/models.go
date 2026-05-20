package domain

type PricingSnapshot struct {
	InputPriceMicroUSDPer1KTokens  int64  `json:"input_price_micro_usd_per_1k_tokens"`
	OutputPriceMicroUSDPer1KTokens int64  `json:"output_price_micro_usd_per_1k_tokens"`
	RoundingMode                   string `json:"rounding_mode"`
}

type CostCalculation struct {
	Currency        string          `json:"currency"`
	InputCostMicro  int64           `json:"input_cost_micro"`
	OutputCostMicro int64           `json:"output_cost_micro"`
	TotalCostMicro  int64           `json:"total_cost_micro"`
	PricingSnapshot PricingSnapshot `json:"pricing_snapshot"`
}

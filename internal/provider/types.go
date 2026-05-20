package provider

import contract "github.com/tep/llm-cost-gateway/internal/provider/contract"

type ProviderConfig = contract.ProviderConfig
type ModelConfig = contract.ModelConfig
type ChatMessage = contract.ChatMessage
type ChatRequest = contract.ChatRequest
type Usage = contract.Usage
type ChatResponse = contract.ChatResponse
type Adapter = contract.Adapter
type AdapterError = contract.Error

var NewAdapterError = contract.NewError
var ErrorCode = contract.ErrorCode

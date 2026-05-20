package contract

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

type ProviderConfig struct {
	ID        uuid.UUID
	Name      string
	Type      string
	BaseURL   string
	APIKey    string
	TimeoutMS int32
}

type ModelConfig struct {
	ID                uuid.UUID
	ProviderModelName string
	DisplayName       string
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	RequestID   string
	OrgID       uuid.UUID
	Provider    ProviderConfig
	Model       ModelConfig
	Messages    []ChatMessage
	Temperature *float64
	MaxTokens   *int
	Stream      bool
}

type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

type ChatResponse struct {
	ProviderResponseID string
	Content            string
	Role               string
	FinishReason       string
	Usage              Usage
	LatencyMS          int64
	RawUsage           map[string]any
}

type Adapter interface {
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
}

type Error struct {
	Code    string
	Message string
	Cause   error
}

func NewError(code string, message string, cause error) *Error {
	return &Error{
		Code:    code,
		Message: message,
		Cause:   cause,
	}
}

func (e *Error) Error() string {
	if e.Cause == nil {
		return e.Message
	}
	return fmt.Sprintf("%s: %v", e.Message, e.Cause)
}

func (e *Error) Unwrap() error {
	return e.Cause
}

func ErrorCode(err error) string {
	var providerErr *Error
	if errors.As(err, &providerErr) {
		return providerErr.Code
	}
	return ""
}

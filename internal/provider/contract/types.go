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

type StreamUsage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	Raw              map[string]any
}

type StreamEvent struct {
	Data  string
	Usage *StreamUsage
	Done  bool
}

type ChatStream interface {
	Events() <-chan StreamEvent
	Err() error
	Close() error
}

type Adapter interface {
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
	StreamChat(ctx context.Context, req ChatRequest) (ChatStream, error)
}

type Error struct {
	Code       string
	Message    string
	Cause      error
	StatusCode int
	Retryable  bool
}

func NewError(code string, message string, cause error) *Error {
	return &Error{
		Code:    code,
		Message: message,
		Cause:   cause,
	}
}

func NewStatusError(code string, message string, statusCode int, retryable bool, cause error) *Error {
	return &Error{
		Code:       code,
		Message:    message,
		Cause:      cause,
		StatusCode: statusCode,
		Retryable:  retryable,
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

func StatusCode(err error) int {
	var providerErr *Error
	if errors.As(err, &providerErr) {
		return providerErr.StatusCode
	}
	return 0
}

func Retryable(err error) bool {
	var providerErr *Error
	if errors.As(err, &providerErr) {
		return providerErr.Retryable
	}
	return false
}

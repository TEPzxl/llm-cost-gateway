package gateway

import (
	"context"
	"errors"
	"time"

	"github.com/tep/llm-cost-gateway/internal/domain"
	contract "github.com/tep/llm-cost-gateway/internal/provider/contract"
)

const (
	defaultMaxRetries     = 1
	defaultRetryBackoffMS = 100
)

type RetryPolicy struct {
	MaxRetries int
	Backoff    time.Duration
}

func NewRetryPolicy(maxRetries int, retryBackoffMS int) RetryPolicy {
	return RetryPolicy{
		MaxRetries: maxRetries,
		Backoff:    time.Duration(retryBackoffMS) * time.Millisecond,
	}.Normalize()
}

func (p RetryPolicy) Normalize() RetryPolicy {
	if p.MaxRetries < 0 {
		p.MaxRetries = defaultMaxRetries
	}
	if p.Backoff <= 0 {
		p.Backoff = time.Duration(defaultRetryBackoffMS) * time.Millisecond
	}
	return p
}

func (p RetryPolicy) ShouldRetry(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	if contract.Retryable(err) {
		return true
	}
	switch contract.ErrorCode(err) {
	case domain.CodeProviderTimeout, domain.CodeProviderUnavailable:
		return true
	default:
		return false
	}
}

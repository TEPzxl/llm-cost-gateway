package gateway

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tep/llm-cost-gateway/internal/domain"
	contract "github.com/tep/llm-cost-gateway/internal/provider/contract"
)

func TestRetryPolicyNormalize(t *testing.T) {
	policy := RetryPolicy{MaxRetries: -1}.Normalize()

	if policy.MaxRetries != defaultMaxRetries {
		t.Fatalf("MaxRetries = %d, want %d", policy.MaxRetries, defaultMaxRetries)
	}
	if policy.Backoff != time.Duration(defaultRetryBackoffMS)*time.Millisecond {
		t.Fatalf("Backoff = %s, want %dms", policy.Backoff, defaultRetryBackoffMS)
	}
}

func TestRetryPolicyShouldRetry(t *testing.T) {
	policy := RetryPolicy{}.Normalize()
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "retryable contract error",
			err:  contract.NewStatusError(domain.CodeProviderError, "rate limited", 429, true, nil),
			want: true,
		},
		{
			name: "provider timeout",
			err:  contract.NewError(domain.CodeProviderTimeout, "timeout", nil),
			want: true,
		},
		{
			name: "provider unavailable",
			err:  contract.NewError(domain.CodeProviderUnavailable, "unavailable", nil),
			want: true,
		},
		{
			name: "usage missing",
			err:  contract.NewError(domain.CodeUsageMissing, "usage missing", nil),
			want: false,
		},
		{
			name: "context canceled",
			err:  context.Canceled,
			want: false,
		},
		{
			name: "plain error",
			err:  errors.New("plain error"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := policy.ShouldRetry(tt.err); got != tt.want {
				t.Fatalf("ShouldRetry = %v, want %v", got, tt.want)
			}
		})
	}
}

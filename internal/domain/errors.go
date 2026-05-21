package domain

import "fmt"

const (
	CodeInvalidRequest       = "invalid_request"
	CodeStreamNotSupported   = "stream_not_supported"
	CodeUnauthorized         = "unauthorized"
	CodeForbidden            = "forbidden"
	CodeNotFound             = "not_found"
	CodeRouteNotFound        = "route_not_found"
	CodeConflict             = "conflict"
	CodeRateLimitExceeded    = "rate_limit_exceeded"
	CodeBudgetExceeded       = "budget_exceeded"
	CodeAPIKeyQuotaExceeded  = "api_key_quota_exceeded"
	CodeContentPolicyBlocked = "content_policy_blocked"
	CodeCostAnomalyBlocked   = "cost_anomaly_blocked"
	CodeProviderError        = "provider_error"
	CodeProviderUnavailable  = "provider_unavailable"
	CodeProviderTimeout      = "provider_timeout"
	CodeUsageMissing         = "usage_missing"
	CodeInternalError        = "internal_error"
)

type Error struct {
	Status  int
	Code    string
	Message string
	Cause   error
}

func NewError(status int, code string, message string) *Error {
	return &Error{
		Status:  status,
		Code:    code,
		Message: message,
	}
}

func WrapError(status int, code string, message string, cause error) *Error {
	return &Error{
		Status:  status,
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

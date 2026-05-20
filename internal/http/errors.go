package httpapi

import (
	"net/http"

	"github.com/tep/llm-cost-gateway/internal/domain"
)

func InvalidRequest(message string) *domain.Error {
	return domain.NewError(http.StatusBadRequest, domain.CodeInvalidRequest, message)
}

func Unauthorized(message string) *domain.Error {
	return domain.NewError(http.StatusUnauthorized, domain.CodeUnauthorized, message)
}

func NotFound(message string) *domain.Error {
	return domain.NewError(http.StatusNotFound, domain.CodeNotFound, message)
}

func Conflict(message string) *domain.Error {
	return domain.NewError(http.StatusConflict, domain.CodeConflict, message)
}

func InternalError(message string) *domain.Error {
	return domain.NewError(http.StatusInternalServerError, domain.CodeInternalError, message)
}

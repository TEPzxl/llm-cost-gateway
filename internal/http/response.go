package httpapi

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tep/llm-cost-gateway/internal/domain"
	"github.com/tep/llm-cost-gateway/internal/http/middleware"
)

type ErrorEnvelope struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

func RespondJSON(c *gin.Context, status int, body any) {
	c.JSON(status, body)
}

func RespondError(c *gin.Context, err error) {
	appErr := normalizeError(err)
	c.JSON(appErr.Status, ErrorEnvelope{
		Error: ErrorBody{
			Code:      appErr.Code,
			Message:   appErr.Message,
			RequestID: middleware.RequestIDFromContext(c),
		},
	})
}

func normalizeError(err error) *domain.Error {
	var appErr *domain.Error
	if errors.As(err, &appErr) {
		return appErr
	}

	return domain.NewError(http.StatusInternalServerError, domain.CodeInternalError, "internal server error")
}

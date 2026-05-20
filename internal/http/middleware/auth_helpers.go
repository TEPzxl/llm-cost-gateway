package middleware

import (
	"crypto/hmac"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tep/llm-cost-gateway/internal/domain"
)

func bearerToken(header string) (string, bool) {
	if !strings.HasPrefix(header, "Bearer ") {
		return "", false
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	return token, token != ""
}

func constantTimeEqual(a string, b string) bool {
	return hmac.Equal([]byte(a), []byte(b))
}

func abortUnauthorized(c *gin.Context) {
	abortWithError(c, http.StatusUnauthorized, domain.CodeUnauthorized, "unauthorized")
}

func abortWithError(c *gin.Context, status int, code string, message string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"code":       code,
			"message":    message,
			"request_id": RequestIDFromContext(c),
		},
	})
	c.Abort()
}

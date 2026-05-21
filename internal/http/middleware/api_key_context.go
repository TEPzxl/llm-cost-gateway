package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tep/llm-cost-gateway/internal/auth"
	"github.com/tep/llm-cost-gateway/internal/domain"
)

const apiKeyPrincipalKey = "api_key_principal"

func SetAPIKeyPrincipal(c *gin.Context, principal auth.APIKeyPrincipal) {
	c.Set(apiKeyPrincipalKey, principal)
}

func APIKeyPrincipalFromContext(c *gin.Context) (auth.APIKeyPrincipal, bool) {
	value, ok := c.Get(apiKeyPrincipalKey)
	if !ok {
		return auth.APIKeyPrincipal{}, false
	}
	principal, ok := value.(auth.APIKeyPrincipal)
	return principal, ok
}

func RequireAPIKeyScope(scope string) gin.HandlerFunc {
	return func(c *gin.Context) {
		principal, ok := APIKeyPrincipalFromContext(c)
		if !ok {
			abortUnauthorized(c)
			return
		}
		if !principal.HasScope(scope) {
			abortWithError(c, http.StatusForbidden, domain.CodeForbidden, "forbidden")
			return
		}
		c.Next()
	}
}

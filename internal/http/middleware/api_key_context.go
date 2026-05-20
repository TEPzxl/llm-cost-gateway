package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/tep/llm-cost-gateway/internal/auth"
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

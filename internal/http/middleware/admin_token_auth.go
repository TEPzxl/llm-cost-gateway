package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/tep/llm-cost-gateway/internal/auth"
)

const adminTokenPrincipalKey = "admin_token_principal"

func AdminTokenAuth(authenticator auth.AdminTokenAuthenticator) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := bearerToken(c.GetHeader("Authorization"))
		if !ok {
			abortUnauthorized(c)
			return
		}

		principal, err := authenticator.Authenticate(c.Request.Context(), token)
		if err != nil {
			abortUnauthorized(c)
			return
		}

		c.Set(adminTokenPrincipalKey, principal)
		c.Next()
	}
}

func AdminTokenPrincipalFromContext(c *gin.Context) (auth.AdminTokenPrincipal, bool) {
	value, ok := c.Get(adminTokenPrincipalKey)
	if !ok {
		return auth.AdminTokenPrincipal{}, false
	}
	principal, ok := value.(auth.AdminTokenPrincipal)
	return principal, ok
}

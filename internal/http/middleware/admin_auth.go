package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tep/llm-cost-gateway/internal/auth"
)

func AdminAuth(adminTokens auth.AdminTokenAuthenticator, sessions auth.AdminTokenAuthenticator) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := bearerToken(c.GetHeader("Authorization"))
		if !ok {
			abortUnauthorized(c)
			return
		}

		var (
			principal auth.AdminTokenPrincipal
			err       error
		)
		switch {
		case strings.HasPrefix(token, auth.AdminTokenPlainPrefix):
			principal, err = adminTokens.Authenticate(c.Request.Context(), token)
		case strings.HasPrefix(token, auth.SessionPlainPrefix):
			principal, err = sessions.Authenticate(c.Request.Context(), token)
		default:
			principal, err = adminTokens.Authenticate(c.Request.Context(), token)
			if err != nil {
				principal, err = sessions.Authenticate(c.Request.Context(), token)
			}
		}
		if err != nil {
			abortUnauthorized(c)
			return
		}

		c.Set(adminTokenPrincipalKey, principal)
		c.Next()
	}
}

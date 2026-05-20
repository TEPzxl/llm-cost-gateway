package middleware

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/tep/llm-cost-gateway/internal/auth"
)

type APIKeyAuthenticator interface {
	Authenticate(context.Context, string) (auth.APIKeyPrincipal, error)
}

func GatewayAPIKeyAuth(authenticator APIKeyAuthenticator) gin.HandlerFunc {
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

		SetAPIKeyPrincipal(c, principal)
		c.Next()
	}
}

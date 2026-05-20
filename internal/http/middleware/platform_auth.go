package middleware

import "github.com/gin-gonic/gin"

func PlatformAuth(expectedToken string) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := bearerToken(c.GetHeader("Authorization"))
		if !ok || !constantTimeEqual(token, expectedToken) {
			abortUnauthorized(c)
			return
		}

		c.Next()
	}
}

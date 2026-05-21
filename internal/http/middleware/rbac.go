package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tep/llm-cost-gateway/internal/auth"
	"github.com/tep/llm-cost-gateway/internal/domain"
)

type AdminPermission func(auth.AdminTokenPrincipal) bool

func RequireAdminPermission(allowed AdminPermission) gin.HandlerFunc {
	return func(c *gin.Context) {
		principal, ok := AdminTokenPrincipalFromContext(c)
		if !ok {
			abortUnauthorized(c)
			return
		}
		if allowed == nil || !allowed(principal) {
			abortWithError(c, http.StatusForbidden, domain.CodeForbidden, "forbidden")
			return
		}
		c.Next()
	}
}

func CanManageMembers(principal auth.AdminTokenPrincipal) bool {
	return principal.CanManageMembers()
}

func CanViewMembers(principal auth.AdminTokenPrincipal) bool {
	return principal.CanViewMembers()
}

func CanManageTokens(principal auth.AdminTokenPrincipal) bool {
	return principal.CanManageTokens()
}

func CanManageConfiguration(principal auth.AdminTokenPrincipal) bool {
	return principal.CanManageConfiguration()
}

func CanView(principal auth.AdminTokenPrincipal) bool {
	return principal.CanView()
}

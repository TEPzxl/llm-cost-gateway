package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/tep/llm-cost-gateway/internal/auth"
	"github.com/tep/llm-cost-gateway/internal/domain"
)

func TestRequireAPIKeyScopeAllowsMatchingScope(t *testing.T) {
	router := apiKeyScopeTestRouter(auth.APIKeyPrincipal{Scopes: []string{auth.APIKeyScopeChatCompletions}})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusNoContent, recorder.Body.String())
	}
}

func TestRequireAPIKeyScopeRejectsMissingScope(t *testing.T) {
	router := apiKeyScopeTestRouter(auth.APIKeyPrincipal{Scopes: []string{"embeddings.create"}})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))

	assertErrorResponse(t, recorder, http.StatusForbidden, domain.CodeForbidden)
}

func apiKeyScopeTestRouter(principal auth.APIKeyPrincipal) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID())
	router.Use(func(c *gin.Context) {
		SetAPIKeyPrincipal(c, principal)
		c.Next()
	})
	router.Use(RequireAPIKeyScope(auth.APIKeyScopeChatCompletions))
	router.POST("/v1/chat/completions", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	return router
}

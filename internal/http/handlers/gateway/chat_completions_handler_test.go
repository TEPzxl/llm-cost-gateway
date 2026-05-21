package gateway

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tep/llm-cost-gateway/internal/auth"
	"github.com/tep/llm-cost-gateway/internal/domain"
	"github.com/tep/llm-cost-gateway/internal/http/middleware"
)

func TestChatCompletionsRejectsTooLargeRequestBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewChatCompletionsHandler(nil)
	router := gin.New()
	router.Use(middleware.RequestID())
	router.Use(middleware.RequestBodyLimit(8))
	router.Use(func(c *gin.Context) {
		middleware.SetAPIKeyPrincipal(c, auth.APIKeyPrincipal{
			OrgID:    uuid.New(),
			APIKeyID: uuid.New(),
			Scopes:   []string{auth.APIKeyScopeChatCompletions},
			RPMLimit: 60,
		})
		c.Next()
	})
	router.POST("/v1/chat/completions", handler.Create)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"fast-chat","messages":[{"role":"user","content":"hello"}]}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	assertGatewayError(t, rec, http.StatusRequestEntityTooLarge, domain.CodeInvalidRequest)
}

func assertGatewayError(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantCode string) {
	t.Helper()
	if rec.Code != wantStatus {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, wantStatus, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"code":"`+wantCode+`"`) {
		t.Fatalf("body = %s, want error code %s", rec.Body.String(), wantCode)
	}
}

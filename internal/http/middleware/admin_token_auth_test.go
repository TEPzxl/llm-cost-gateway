package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tep/llm-cost-gateway/internal/auth"
)

func TestAdminTokenAuthInjectsPrincipal(t *testing.T) {
	gin.SetMode(gin.TestMode)

	orgID := uuid.New()
	tokenID := uuid.New()
	authenticator := &stubAdminTokenAuthenticator{
		principal: auth.AdminTokenPrincipal{
			OrgID:        orgID,
			AdminTokenID: tokenID,
			Scopes:       []string{"admin:*"},
		},
	}

	router := gin.New()
	router.Use(AdminTokenAuth(authenticator))
	router.GET("/admin/me", func(c *gin.Context) {
		principal, ok := AdminTokenPrincipalFromContext(c)
		if !ok {
			t.Fatal("admin token principal missing from context")
		}
		if principal.OrgID != orgID {
			t.Fatalf("principal org id = %s, want %s", principal.OrgID, orgID)
		}
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/admin/me", nil)
	req.Header.Set("Authorization", "Bearer llmgw_admin_token")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if authenticator.seenToken != "llmgw_admin_token" {
		t.Fatalf("seen token = %q, want llmgw_admin_token", authenticator.seenToken)
	}
}

func TestAdminTokenAuthRejectsInvalidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(RequestID())
	router.Use(AdminTokenAuth(&stubAdminTokenAuthenticator{err: errors.New("invalid token")}))
	router.GET("/admin/me", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/admin/me", nil)
	req.Header.Set("Authorization", "Bearer llmgw_admin_bad")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

type stubAdminTokenAuthenticator struct {
	principal auth.AdminTokenPrincipal
	err       error
	seenToken string
}

func (s *stubAdminTokenAuthenticator) Authenticate(_ context.Context, token string) (auth.AdminTokenPrincipal, error) {
	s.seenToken = token
	if s.err != nil {
		return auth.AdminTokenPrincipal{}, s.err
	}
	return s.principal, nil
}

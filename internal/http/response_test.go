package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/tep/llm-cost-gateway/internal/domain"
	"github.com/tep/llm-cost-gateway/internal/http/middleware"
)

func TestRespondErrorIncludesRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.RequestID())
	router.GET("/error", func(c *gin.Context) {
		RespondError(c, domain.NewError(http.StatusBadRequest, domain.CodeInvalidRequest, "bad input"))
	})

	req := httptest.NewRequest(http.MethodGet, "/error", nil)
	req.Header.Set(middleware.RequestIDHeader, "req-error-123")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}

	var body struct {
		Error struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			RequestID string `json:"request_id"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Error.Code != domain.CodeInvalidRequest {
		t.Fatalf("error code = %q, want %q", body.Error.Code, domain.CodeInvalidRequest)
	}
	if body.Error.Message != "bad input" {
		t.Fatalf("message = %q, want bad input", body.Error.Message)
	}
	if body.Error.RequestID != "req-error-123" {
		t.Fatalf("request_id = %q, want req-error-123", body.Error.RequestID)
	}
}

func TestRespondErrorMapsUnknownErrorToInternalError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.RequestID())
	router.GET("/error", func(c *gin.Context) {
		RespondError(c, errUnknown{})
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/error", nil)

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

type errUnknown struct{}

func (errUnknown) Error() string {
	return "unknown"
}

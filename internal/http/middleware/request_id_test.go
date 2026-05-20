package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequestIDUsesIncomingHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID())
	router.GET("/request-id", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"request_id": RequestIDFromContext(c)})
	})

	req := httptest.NewRequest(http.MethodGet, "/request-id", nil)
	req.Header.Set(RequestIDHeader, "req-test-123")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Header().Get(RequestIDHeader) != "req-test-123" {
		t.Fatalf("response request id header = %q, want req-test-123", rec.Header().Get(RequestIDHeader))
	}

	var body struct {
		RequestID string `json:"request_id"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.RequestID != "req-test-123" {
		t.Fatalf("request id = %q, want req-test-123", body.RequestID)
	}
}

func TestRequestIDGeneratesUUIDWhenMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID())
	router.GET("/request-id", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"request_id": RequestIDFromContext(c)})
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/request-id", nil)

	router.ServeHTTP(rec, req)

	requestID := rec.Header().Get(RequestIDHeader)
	if requestID == "" {
		t.Fatal("response request id header is empty")
	}

	var body struct {
		RequestID string `json:"request_id"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.RequestID != requestID {
		t.Fatalf("body request id = %q, want response header %q", body.RequestID, requestID)
	}
}

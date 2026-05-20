package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestLoggingRecordsRequestFieldsWithoutAuthorization(t *testing.T) {
	gin.SetMode(gin.TestMode)

	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)

	router := gin.New()
	router.Use(RequestID())
	router.Use(Logging(logger))
	router.GET("/healthz", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set(RequestIDHeader, "req-log-123")
	req.Header.Set("Authorization", "Bearer secret-token")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("logged entries = %d, want 1", len(entries))
	}

	fields := entries[0].ContextMap()
	if fields["request_id"] != "req-log-123" {
		t.Fatalf("request_id field = %v, want req-log-123", fields["request_id"])
	}
	if fields["method"] != http.MethodGet {
		t.Fatalf("method field = %v, want GET", fields["method"])
	}
	if fields["path"] != "/healthz" {
		t.Fatalf("path field = %v, want /healthz", fields["path"])
	}
	if fields["status"] != int64(http.StatusNoContent) {
		t.Fatalf("status field = %v, want 204", fields["status"])
	}
	if _, ok := fields["authorization"]; ok {
		t.Fatal("log includes authorization field")
	}
	if _, ok := fields["Authorization"]; ok {
		t.Fatal("log includes Authorization field")
	}
	for key, value := range fields {
		if value == "Bearer secret-token" || value == "secret-token" {
			t.Fatalf("log field %q leaked token", key)
		}
	}
}

package observability

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
)

func TestNewTracerProviderDisabledUsesNoop(t *testing.T) {
	provider, err := NewTracerProvider(context.Background(), TracingConfig{Enabled: false})
	if err != nil {
		t.Fatalf("NewTracerProvider returned error: %v", err)
	}
	if provider != nil {
		t.Fatal("provider should be nil when tracing is disabled")
	}

	_, span := otel.Tracer("test").Start(context.Background(), "disabled")
	if span.IsRecording() {
		t.Fatal("disabled tracing span should not record")
	}
	span.End()
}

func TestNewTracerProviderEnabledCreatesProvider(t *testing.T) {
	provider, err := NewTracerProvider(context.Background(), TracingConfig{
		Enabled:     true,
		Endpoint:    "localhost:4318",
		Insecure:    true,
		ServiceName: "test-gateway",
	})
	if err != nil {
		t.Fatalf("NewTracerProvider returned error: %v", err)
	}
	if provider == nil {
		t.Fatal("provider should not be nil when tracing is enabled")
	}
	if err := provider.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown returned error: %v", err)
	}
}

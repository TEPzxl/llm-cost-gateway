package gateway

import (
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

func endTracingSpan(span trace.Span, status string, errorCode string, err error) {
	span.SetAttributes(attribute.String("status", status))
	if err != nil {
		recordTracingError(span, errorCode, err)
	}
	span.End()
}

func recordTracingError(span trace.Span, errorCode string, err error) {
	if errorCode != "" {
		span.SetAttributes(attribute.String("error_code", errorCode))
	}
	if err != nil {
		span.RecordError(err)
	}
	span.SetStatus(codes.Error, errorCode)
}

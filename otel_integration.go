// Copyright (c) 2025 Nguyễn Thanh Phương
// This source code is licensed under the MIT License found in the LICENSE file.

// Package unologger provides a flexible and feature-rich logging library for Go applications.
// This file contains the integration logic for OpenTelemetry (OTel).
// This integration allows the logger to automatically extract trace and span IDs
// from the context and include them in log entries, which is essential for
// correlating logs with traces in distributed systems.

package unologger

import (
	"context"

	"go.opentelemetry.io/otel/trace"
)

// extractOTelTraceID is an internal helper that safely extracts the OTel trace ID
// from a context. It returns the trace ID as a string if a valid span is found,
// otherwise it returns an empty string.
func extractOTelTraceID(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if span == nil {
		return ""
	}
	spanContext := span.SpanContext()
	if !spanContext.HasTraceID() {
		return ""
	}
	return spanContext.TraceID().String()
}

// extractOTelSpanID is an internal helper that safely extracts the OTel span ID
// from a context. It returns the span ID as a string if a valid span is found,
// otherwise it returns an empty string.
func extractOTelSpanID(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if span == nil {
		return ""
	}
	spanContext := span.SpanContext()
	if !spanContext.HasSpanID() {
		return ""
	}
	return spanContext.SpanID().String()
}

// AttachOTelTrace enriches the given context with trace and span IDs from an
// active OpenTelemetry span, if one exists.
//
// It extracts the trace ID and attaches it using `WithTraceID`. The span ID is
// attached as a custom attribute with the key "span_id". This function is
// automatically called by the logger's core `log` method if the `EnableOTel`
// configuration flag is set to true.
func AttachOTelTrace(ctx context.Context) context.Context {
	tid := extractOTelTraceID(ctx)
	if tid == "" {
		// If there's no trace ID, there's nothing to attach.
		return ctx
	}

	// Attach the trace ID to the context for direct access.
	ctx = WithTraceID(ctx, tid)

	// Also attach the span ID as a field for more detailed correlation.
	if sid := extractOTelSpanID(ctx); sid != "" {
		ctx = WithAttrs(ctx, Fields{"span_id": sid})
	}
	return ctx
}

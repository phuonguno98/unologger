// Copyright (c) 2025 Nguyễn Thanh Phương
// This source code is licensed under the MIT License found in the LICENSE file.

// Package unologger provides a flexible and feature-rich logging library for Go applications.
// This file provides integration functions with OpenTelemetry (OTel) to automatically
// extract trace IDs and span IDs from the `context.Context` and attach them to log entries.
// This integration is crucial for correlating logs with tracing data, enabling better
// observability and debugging in distributed systems.

package unologger

import (
	"context"

	"go.opentelemetry.io/otel/trace"
)

// extractOTelTraceID attempts to extract the trace ID from the OpenTelemetry span
// context within the provided `context.Context`.
// It returns the trace ID as a string if found and valid, otherwise returns an empty string.
func extractOTelTraceID(ctx context.Context) string {
	// Retrieve the current span from the context.
	if span := trace.SpanFromContext(ctx); span != nil {
		sc := span.SpanContext() // Get the SpanContext.
		if sc.HasTraceID() {     // Check if the SpanContext has a valid TraceID.
			return sc.TraceID().String() // Return the string representation of the TraceID.
		}
	}
	return "" // No valid TraceID found in the context.
}

// extractOTelSpanID attempts to extract the span ID from the OpenTelemetry span
// context within the provided `context.Context`.
// It returns the span ID as a string if found and valid, otherwise returns an empty string.
func extractOTelSpanID(ctx context.Context) string {
	// Retrieve the current span from the context.
	if span := trace.SpanFromContext(ctx); span != nil {
		sc := span.SpanContext() // Get the SpanContext.
		if sc.HasSpanID() {      // Check if the SpanContext has a valid SpanID.
			return sc.SpanID().String() // Return the string representation of the SpanID.
		}
	}
	return "" // No valid SpanID found in the context.
}

// AttachOTelTrace attaches the OpenTelemetry trace ID and span ID (if available)
// from the provided `context.Context` to the log context.
// If no valid trace ID is found in the OTel context, the original context is returned unchanged.
// The trace ID is attached using WithTraceID, and the span ID is attached as a custom attribute
// named "span_id" using WithAttrs, allowing hooks or writers to utilize it.
func AttachOTelTrace(ctx context.Context) context.Context {
	tid := extractOTelTraceID(ctx) // Extract Trace ID.
	sid := extractOTelSpanID(ctx)  // Extract Span ID.

	if tid == "" {
		return ctx // If no valid Trace ID, return the original context.
	}

	// Attach the Trace ID to the log context.
	ctx = WithTraceID(ctx, tid)

	if sid != "" {
		// Attach the Span ID as a custom attribute.
		ctx = WithAttrs(ctx, Fields{"span_id": sid})
	}
	return ctx
}

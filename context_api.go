// Copyright (c) 2025 Nguyễn Thanh Phương
// This source code is licensed under the MIT License found in the LICENSE file.

// Package unologger provides a flexible and feature-rich logging library for Go applications.
// This file offers utility functions for managing and propagating logging-related metadata
// within a `context.Context`. It allows attaching and retrieving Logger instances,
// module names, trace IDs, flow IDs, and custom attributes to the context,
// facilitating context-aware logging throughout an application.
package unologger

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"time"
)

// WithLogger attaches a *Logger instance to the provided context and returns the new context.
// This allows specific logger configurations to be propagated down the call chain.
func WithLogger(ctx context.Context, l *Logger) context.Context {
	return context.WithValue(ctx, ctxLoggerKey{}, l)
}

// LoggerFromContext attempts to retrieve a *Logger instance from the given context.
// It returns the Logger and a boolean indicating whether a Logger was found in the context.
func LoggerFromContext(ctx context.Context) (*Logger, bool) {
	l, ok := ctx.Value(ctxLoggerKey{}).(*Logger)
	return l, ok
}

// WithModule attaches a module name to the context and returns a new LoggerWithCtx.
// This is a convenient way to categorize log entries by the originating application module.
// If the global logger is not initialized, it will be initialized with default settings.
func WithModule(ctx context.Context, module string) LoggerWithCtx {
	ensureInit() // Ensure global logger is initialized if not already.
	ctx = context.WithValue(ctx, ctxModuleKey, module)
	return GetLogger(ctx) // Retrieve LoggerWithCtx with the updated context.
}

// WithTraceID attaches or overrides a trace ID in the provided context.
// Trace IDs are crucial for correlating log entries across distributed services.
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, ctxTraceIDKey, traceID)
}

// WithFlowID attaches a flow ID to the provided context.
// Flow IDs can be used for custom request tracking or business process correlation.
func WithFlowID(ctx context.Context, flowID string) context.Context {
	return context.WithValue(ctx, ctxFlowIDKey, flowID)
}

// WithAttrs attaches additional key-value attributes (Fields) to the provided context.
// If a key already exists in the context's attributes, its value will be overwritten.
// This allows enriching log entries with dynamic, context-specific data.
func WithAttrs(ctx context.Context, attrs Fields) context.Context {
	if attrs == nil {
		return ctx
	}
	// Retrieve existing fields from context, if any.
	existing, _ := ctx.Value(ctxFieldsKey).(Fields) // Assuming ctxFieldsKey is defined in logger_types.go
	// Create a new map to avoid modifying the original context's map directly.
	newMap := make(Fields, len(existing)+len(attrs))
	for k, v := range existing {
		newMap[k] = v
	}
	for k, v := range attrs {
		newMap[k] = v
	}
	return context.WithValue(ctx, ctxFieldsKey, newMap)
}

// EnsureTraceIDCtx ensures that the provided context contains a trace ID.
// It prioritizes extracting a trace ID from OpenTelemetry if enabled and available.
// If no trace ID is found, a new RFC 4122 compliant UUID v4 is generated and attached.
func EnsureTraceIDCtx(ctx context.Context) context.Context {
	// Check if a trace ID already exists in the context.
	if id, ok := ctx.Value(ctxTraceIDKey).(string); ok && id != "" {
		return ctx
	}
	// Check for OpenTelemetry trace ID if OTEL integration is enabled.
	globalMu.RLock()
	l := globalLogger
	globalMu.RUnlock()
	if l != nil && l.enableOTEL.Load() {
		if tid := extractOTELTraceID(ctx); tid != "" { // Assuming extractOTELTraceID exists
			return context.WithValue(ctx, ctxTraceIDKey, tid)
		}
	}
	// If no trace ID found, generate a new UUID.
	return context.WithValue(ctx, ctxTraceIDKey, newUUID())
}

// newUUID generates a new RFC 4122 compliant UUID v4 using crypto/rand.
// This function does not require external libraries for UUID generation.
func newUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// In a real application, you might log this error or return an error.
		// For simplicity in this library, we panic on failure to read random bytes.
		panic(err)
	}
	// Set version (4) and variant bits according to RFC 4122.
	b[6] = (b[6] & 0x0f) | 0x40 // Version 4
	b[8] = (b[8] & 0x3f) | 0x80 // RFC 4122 variant
	return hex.EncodeToString(b[0:4]) + "-" +
		hex.EncodeToString(b[4:6]) + "-" +
		hex.EncodeToString(b[6:8]) + "-" +
		hex.EncodeToString(b[8:10]) + "-" +
		hex.EncodeToString(b[10:16])
}

// GetLogger retrieves a LoggerWithCtx from the provided context.
// If no Logger is found in the context, it falls back to the global logger.
// It also ensures that a module name is present in the context, defaulting to "unknown" if not set.
func GetLogger(ctx context.Context) LoggerWithCtx {
	ensureInit() // Ensure global logger is initialized if not already.
	var base *Logger
	// Try to get a specific logger from the context.
	if l, ok := ctx.Value(ctxLoggerKey{}).(*Logger); ok && l != nil {
		base = l
	} else {
		// Fallback to the global logger.
		globalMu.RLock()
		base = globalLogger
		globalMu.RUnlock()
	}
	// Ensure module name is present in the context.
	module, _ := ctx.Value(ctxModuleKey).(string)
	if module == "" {
		module = "unknown"
	}
	ctx = context.WithValue(ctx, ctxModuleKey, module)
	return LoggerWithCtx{l: base, ctx: ctx}
}

// Context returns the underlying context.Context associated with this LoggerWithCtx.
// This allows external code to retrieve the context for further propagation or inspection.
func (lw LoggerWithCtx) Context() context.Context {
	return lw.ctx
}

// Debug logs a message at DEBUG level using the LoggerWithCtx's internal Logger
// and its associated context.
func (lw LoggerWithCtx) Debug(format string, args ...interface{}) {
	lw.l.log(lw.ctx, DEBUG, format, args...)
}

// Info logs a message at INFO level using the LoggerWithCtx's internal Logger
// and its associated context.
func (lw LoggerWithCtx) Info(format string, args ...interface{}) {
	lw.l.log(lw.ctx, INFO, format, args...)
}

// Warn logs a message at WARN level using the LoggerWithCtx's internal Logger
// and its associated context.
func (lw LoggerWithCtx) Warn(format string, args ...interface{}) {
	lw.l.log(lw.ctx, WARN, format, args...)
}

// Error logs a message at ERROR level using the LoggerWithCtx's internal Logger
// and its associated context.
func (lw LoggerWithCtx) Error(format string, args ...interface{}) {
	lw.l.log(lw.ctx, ERROR, format, args...)
}

// Fatal logs a message at FATAL level, attempts to close the logger,
// and then exits the process with status 1.
// It uses the LoggerWithCtx's internal Logger and its associated context.
func (lw LoggerWithCtx) Fatal(format string, args []interface{}, fields Fields) {
	lw.l.log(lw.ctx, FATAL, format, args, fields)
	_ = CloseDetached(lw.l, 2*time.Second) // Assuming CloseDetached exists
	os.Exit(1)
}
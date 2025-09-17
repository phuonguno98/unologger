// Copyright (c) 2025 Nguyễn Thanh Phương
// This source code is licensed under the MIT License found in the LICENSE file.

// Package unologger provides a flexible and feature-rich logging library for Go applications.
// This file defines context-aware helpers for propagating logging metadata. It provides
// functions to attach and retrieve loggers, modules, trace IDs, and other attributes
// to and from a context.Context, enabling seamless logging context across function
// calls and goroutines.

package unologger

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"time"
)

// WithLogger attaches a specific *Logger instance to the context.
// This is an advanced feature for when a non-global logger instance needs to be
// propagated through a specific request or goroutine chain.
func WithLogger(ctx context.Context, l *Logger) context.Context {
	return context.WithValue(ctx, ctxLoggerKey, l)
}

// LoggerFromContext retrieves a *Logger instance from the context, if one exists.
// It returns the logger and a boolean indicating if it was found.
func LoggerFromContext(ctx context.Context) (*Logger, bool) {
	l, ok := ctx.Value(ctxLoggerKey).(*Logger)
	return l, ok
}

// WithModule returns a new LoggerWithCtx that includes the specified module name.
// This is the standard way to create a logger for a specific application component.
// It ensures the global logger is initialized if it hasn't been already.
func WithModule(ctx context.Context, module string) LoggerWithCtx {
	ensureInit() // Ensure global logger is available.
	ctx = context.WithValue(ctx, ctxModuleKey, module)
	return GetLogger(ctx) // Return a new context-aware logger.
}

// WithTraceID returns a new context with the provided trace ID attached.
// Trace IDs are essential for correlating logs across distributed services.
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, ctxTraceIDKey, traceID)
}

// WithFlowID returns a new context with the provided flow ID attached.
// Flow IDs can be used for custom tracking of business processes or requests.
func WithFlowID(ctx context.Context, flowID string) context.Context {
	return context.WithValue(ctx, ctxFlowIDKey, flowID)
}

// WithAttrs returns a new context containing the provided key-value attributes (Fields).
// If the context already contains attributes, the new attributes are merged with the
// existing ones. If a key exists in both, the new value overwrites the old one.
// This allows for enriching log entries with dynamic, request-specific data.
func WithAttrs(ctx context.Context, attrs Fields) context.Context {
	if attrs == nil {
		return ctx
	}
	// Retrieve existing fields from context, if any.
	existing, _ := ctx.Value(ctxFieldsKey).(Fields) // Assuming ctxFieldsKey is defined in logger_types.go
	// Create a new map to ensure immutability.
	newMap := make(Fields, len(existing)+len(attrs))
	for k, v := range existing {
		newMap[k] = v
	}
	for k, v := range attrs {
		newMap[k] = v
	}
	return context.WithValue(ctx, ctxFieldsKey, newMap)
}

// EnsureTraceIDCtx ensures a trace ID is present in the context.
//
// It checks for a trace ID in the following order:
//  1. An existing trace ID already in the context.
//  2. A trace ID from an OpenTelemetry span, if OTel integration is enabled.
//  3. A newly generated UUID v4 if none is found.
//
// This guarantees that logs will have a trace ID for correlation.
func EnsureTraceIDCtx(ctx context.Context) context.Context {
	// 1. Check if a trace ID already exists.
	if id, ok := ctx.Value(ctxTraceIDKey).(string); ok && id != "" {
		return ctx
	}
	// 2. Check for OpenTelemetry trace ID.
	globalMu.RLock()
	l := globalLogger
	globalMu.RUnlock()
	if l != nil && l.enableOTel.Load() {
		if tid := extractOTelTraceID(ctx); tid != "" { // Assuming extractOTELTraceID exists
			return context.WithValue(ctx, ctxTraceIDKey, tid)
		}
	}
	// 3. Generate a new UUID as a fallback.
	return context.WithValue(ctx, ctxTraceIDKey, newUUID())
}

// newUUID generates a new RFC 4122 compliant UUID v4 using crypto/rand.
// It panics if it fails to read from the random source, as a UUID is considered
// essential for tracing and this failure is a critical, non-recoverable error.
func newUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("unologger: failed to read random bytes for UUID generation: " + err.Error())
	}
	// Set version (4) and variant (RFC 4122).
	b[6] = (b[6] & 0x0f) | 0x40 // Version 4
	b[8] = (b[8] & 0x3f) | 0x80 // Variant RFC 4122
	return hex.EncodeToString(b[0:4]) + "-" +
		hex.EncodeToString(b[4:6]) + "-" +
		hex.EncodeToString(b[6:8]) + "-" +
		hex.EncodeToString(b[8:10]) + "-" +
		hex.EncodeToString(b[10:16])
}

// GetLogger retrieves a LoggerWithCtx from the context.
// If a logger is not found in the context, it falls back to the global logger.
// It also ensures a module name is present, defaulting to "unknown" if not set.
func GetLogger(ctx context.Context) LoggerWithCtx {
	ensureInit() // Ensure global logger is available.
	var base *Logger
	// Prefer the logger instance from the context if available.
	if l, ok := ctx.Value(ctxLoggerKey).(*Logger); ok && l != nil {
		base = l
	} else {
		// Fallback to the global logger.
		globalMu.RLock()
		base = globalLogger
		globalMu.RUnlock()
	}
	// Ensure module name is present for categorization.
	if module, ok := ctx.Value(ctxModuleKey).(string); !ok || module == "" {
		ctx = context.WithValue(ctx, ctxModuleKey, "unknown")
	}
	return LoggerWithCtx{l: base, ctx: ctx}
}

// Context returns the underlying context.Context of the LoggerWithCtx.
// This allows the context to be passed along or inspected further.
func (lw LoggerWithCtx) Context() context.Context {
	return lw.ctx
}

// WithAttrs returns a new LoggerWithCtx with additional structured fields (attributes)
// in its context.
func (lw LoggerWithCtx) WithAttrs(attrs Fields) LoggerWithCtx {
	lw.ctx = WithAttrs(lw.ctx, attrs)
	return lw
}

// Debug logs a formatted message at DEBUG level using the logger's context.
func (lw LoggerWithCtx) Debug(format string, args ...interface{}) {
	lw.l.log(lw.ctx, DEBUG, format, args...)
}

// Info logs a formatted message at INFO level using the logger's context.
func (lw LoggerWithCtx) Info(format string, args ...interface{}) {
	lw.l.log(lw.ctx, INFO, format, args...)
}

// Warn logs a formatted message at WARN level using the logger's context.
func (lw LoggerWithCtx) Warn(format string, args ...interface{}) {
	lw.l.log(lw.ctx, WARN, format, args...)
}

// Error logs a formatted message at ERROR level using the logger's context.
func (lw LoggerWithCtx) Error(format string, args ...interface{}) {
	lw.l.log(lw.ctx, ERROR, format, args...)
}

// Fatal logs a formatted message at FATAL level, then attempts to flush logs
// and terminates the application with exit code 1.
func (lw LoggerWithCtx) Fatal(format string, args ...interface{}) {
	lw.l.log(lw.ctx, FATAL, format, args...)
	_ = CloseDetached(lw.l, 2*time.Second) // Assuming CloseDetached exists
	os.Exit(1)
}

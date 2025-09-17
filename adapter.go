// Copyright (c) 2025 Nguyễn Thanh Phương
// This source code is licensed under the MIT License found in the LICENSE file.

// Package unologger provides a flexible and feature-rich logging library for Go applications.
// This file defines a logging Adapter, which provides a simplified, context-unaware
// interface for external packages. It wraps a context-aware logger, making it easier
// to integrate with code that does not propagate context.Context.

package unologger

import "context"

// SimpleLogger defines a basic logging interface with common log levels.
// It is intended for packages that need a simple logger without fatal error handling.
type SimpleLogger interface {
	// Debug logs a message at DEBUG level with support for fmt.Sprintf-style formatting.
	Debug(format string, args ...interface{})
	// Info logs a message at INFO level with support for fmt.Sprintf-style formatting.
	Info(format string, args ...interface{})
	// Warn logs a message at WARN level with support for fmt.Sprintf-style formatting.
	Warn(format string, args ...interface{})
	// Error logs a message at ERROR level with support for fmt.Sprintf-style formatting.
	Error(format string, args ...interface{})
}

// ExtendedLogger extends the SimpleLogger interface with a Fatal method.
// This is suitable for components that need to log critical errors and terminate the application.
type ExtendedLogger interface {
	SimpleLogger
	// Fatal logs a message at FATAL level and then calls os.Exit(1).
	Fatal(format string, args ...interface{})
}

// Adapter wraps a LoggerWithCtx to provide a simplified logging interface that does not
// require passing a context on every call. It is useful for passing a pre-configured
// logger to external modules or legacy code. The adapter is immutable; methods like
// WithModule return a new instance with the updated context.
type Adapter struct {
	lw LoggerWithCtx
}

// NewAdapter creates a new Adapter from a given LoggerWithCtx.
// It panics if the provided logger is nil, as a valid logger is required for operation.
func NewAdapter(lw LoggerWithCtx) *Adapter {
	if lw.l == nil {
		panic("unologger: NewAdapter received LoggerWithCtx with a nil *Logger")
	}
	return &Adapter{lw: lw}
}

// NewAdapterFromContext creates a new Adapter by retrieving a logger from the provided context.
// If no logger is found in the context, it falls back to the global logger instance.
// This serves as a convenient factory method for creating adapters within functions
// where only a context is available.
func NewAdapterFromContext(ctx context.Context) *Adapter {
	return &Adapter{lw: GetLogger(ctx)} // GetLogger handles context extraction or fallback to global.
}

// Context returns the context currently associated with the Adapter.
// This can be used to retrieve values or to create a new logger from it.
func (a *Adapter) Context() context.Context {
	return a.lw.Context()
}

// WithContext returns a new Adapter instance using the provided context.
// The underlying logger remains the same, but the new adapter will use the new context
// for all subsequent log calls.
func (a *Adapter) WithContext(ctx context.Context) *Adapter {
	return &Adapter{lw: LoggerWithCtx{l: a.lw.l, ctx: ctx}}
}

// WithModule returns a new Adapter instance with the specified module name in its context.
// This is a convenient way to categorize logs originating from a specific part of an application.
func (a *Adapter) WithModule(module string) *Adapter {
	lw := WithModule(a.lw.ctx, module) // Use the package-level WithModule function.
	return &Adapter{lw: lw}
}

// WithTraceID returns a new Adapter instance with the specified trace ID in its context.
// This is essential for correlating logs in distributed tracing systems.
func (a *Adapter) WithTraceID(traceID string) *Adapter {
	return a.WithContext(WithTraceID(a.lw.ctx, traceID)) // Use the package-level WithTraceID function.
}

// WithFlowID returns a new Adapter instance with the specified flow ID in its context.
// A flow ID can be used for custom tracking of requests or operations across services.
func (a *Adapter) WithFlowID(flowID string) *Adapter {
	return a.WithContext(WithFlowID(a.lw.ctx, flowID)) // Use the package-level WithFlowID function.
}

// WithAttrs returns a new Adapter instance with additional structured fields (attributes)
// in its context. If a key already exists, its value is overwritten.
func (a *Adapter) WithAttrs(attrs Fields) *Adapter {
	return a.WithContext(WithAttrs(a.lw.ctx, attrs)) // Use the package-level WithAttrs function.
}

// Debug logs a message at DEBUG level using the adapter's embedded context.
func (a *Adapter) Debug(format string, args ...interface{}) {
	a.lw.Debug(format, args...)
}

// Info logs a message at INFO level using the adapter's embedded context.
func (a *Adapter) Info(format string, args ...interface{}) {
	a.lw.Info(format, args...)
}

// Warn logs a message at WARN level using the adapter's embedded context.
func (a *Adapter) Warn(format string, args ...interface{}) {
	a.lw.Warn(format, args...)
}

// Error logs a message at ERROR level using the adapter's embedded context.
func (a *Adapter) Error(format string, args ...interface{}) {
	a.lw.Error(format, args...)
}

// Fatal logs a message at FATAL level and then terminates the application by calling os.Exit(1).
// It uses the adapter's embedded context.
func (a *Adapter) Fatal(format string, args ...interface{}) {
	a.lw.Fatal(format, args, nil)
}

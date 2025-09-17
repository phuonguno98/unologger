// Copyright (c) 2025 Nguyễn Thanh Phương
// This source code is licensed under the MIT License found in the LICENSE file.

// Package unologger provides a flexible and feature-rich logging library for Go applications.
// This file defines an Adapter that wraps a LoggerWithCtx, allowing external packages
// or modules to log messages without explicitly passing a context.Context in every call.
// It supports simplified logging interfaces for easier integration.

package unologger

import "context"

// SimpleLogger is a minimalist logging interface designed for external packages
// that do not require context propagation in every log call.
type SimpleLogger interface {
	// Debug logs a message at DEBUG level.
	Debug(format string, args ...interface{})
	// Info logs a message at INFO level.
	Info(format string, args ...interface{})
	// Warn logs a message at WARN level.
	Warn(format string, args ...interface{})
	// Error logs a message at ERROR level.
	Error(format string, args ...interface{})
}

// ExtendedLogger extends the SimpleLogger interface by adding a Fatal method.
// This interface is suitable for external packages that need to log critical
// errors that should terminate the application.
type ExtendedLogger interface {
	SimpleLogger
	// Fatal logs a message at FATAL level.
	Fatal(format string, args ...interface{})
}

// Adapter wraps a LoggerWithCtx to provide a simplified logging interface.
// It holds a LoggerWithCtx instance, allowing metadata (like module, trace ID)
// to be implicitly carried with log calls without requiring context to be passed
// as an argument to each logging method.
type Adapter struct {
	lw LoggerWithCtx
}

// NewAdapter creates a new Adapter instance from a given LoggerWithCtx.
// It panics if the underlying *Logger within LoggerWithCtx is nil,
// as a valid logger instance is required for the adapter to function.
func NewAdapter(lw LoggerWithCtx) *Adapter {
	if lw.l == nil {
		panic("unologger: NewAdapter received LoggerWithCtx with nil *Logger")
	}
	return &Adapter{lw: lw}
}

// NewAdapterFromContext creates a new Adapter instance by retrieving a LoggerWithCtx
// from the provided context. If no LoggerWithCtx is found in the context,
// it falls back to using the global logger. This is useful for creating
// adapters in functions where only a context is available.
func NewAdapterFromContext(ctx context.Context) *Adapter {
	return &Adapter{lw: GetLogger(ctx)} // GetLogger retrieves LoggerWithCtx from context or global.
}

// Context returns the current context associated with this Adapter.
// This allows inspection or further modification of the context.
func (a *Adapter) Context() context.Context {
	return a.lw.Context()
}

// WithContext returns a new Adapter instance with the provided context.
// The underlying *Logger remains the same, but the context for subsequent
// log calls through this new Adapter will be updated.
func (a *Adapter) WithContext(ctx context.Context) *Adapter {
	return &Adapter{lw: LoggerWithCtx{l: a.lw.l, ctx: ctx}}
}

// WithModule returns a new Adapter instance with the specified module name
// attached to its context. This is a convenient way to categorize logs
// by the originating module.
func (a *Adapter) WithModule(module string) *Adapter {
	lw := WithModule(a.lw.ctx, module) // Use the package-level WithModule function.
	return &Adapter{lw: lw}
}

// WithTraceID returns a new Adapter instance with the specified trace ID
// attached to (or overriding in) its context. This is essential for
// distributed tracing.
func (a *Adapter) WithTraceID(traceID string) *Adapter {
	return a.WithContext(WithTraceID(a.lw.ctx, traceID)) // Use the package-level WithTraceID function.
}

// WithFlowID returns a new Adapter instance with the specified flow ID
// attached to its context. Flow IDs can be used for custom request tracking.
func (a *Adapter) WithFlowID(flowID string) *Adapter {
	return a.WithContext(WithFlowID(a.lw.ctx, flowID)) // Use the package-level WithFlowID function.
}

// WithAttrs returns a new Adapter instance with additional attributes
// attached to its context. If a key already exists, its value will be
// overwritten by the new attributes.
func (a *Adapter) WithAttrs(attrs Fields) *Adapter {
	return a.WithContext(WithAttrs(a.lw.ctx, attrs)) // Use the package-level WithAttrs function.
}

// Debug logs a message at DEBUG level using the Adapter's internal LoggerWithCtx.
// The context associated with the adapter is implicitly used.
func (a *Adapter) Debug(format string, args ...interface{}) {
	a.lw.Debug(format, args...)
}

// Info logs a message at INFO level using the Adapter's internal LoggerWithCtx.
// The context associated with the adapter is implicitly used.
func (a *Adapter) Info(format string, args ...interface{}) {
	a.lw.Info(format, args...)
}

// Warn logs a message at WARN level using the Adapter's internal LoggerWithCtx.
// The context associated with the adapter is implicitly used.
func (a *Adapter) Warn(format string, args ...interface{}) {
	a.lw.Warn(format, args...)
}

// Error logs a message at ERROR level using the Adapter's internal LoggerWithCtx.
// The context associated with the adapter is implicitly used.
func (a *Adapter) Error(format string, args ...interface{}) {
	a.lw.Error(format, args...)
}

// Fatal logs a message at FATAL level and terminates the process.
// It uses the Adapter's internal LoggerWithCtx.Fatal method, which
// attempts to gracefully close the logger before exiting.
func (a *Adapter) Fatal(format string, args []interface{}, fields Fields) {
	a.lw.Fatal(format, args, fields)
}

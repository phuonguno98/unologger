// Copyright (c) 2025 Nguyễn Thanh Phương
// This source code is licensed under the MIT License found in the LICENSE file.

// Package unologger provides a flexible and feature-rich logging library for Go applications.
// This file implements the core logging methods on Logger and LoggerWithCtx.
// It handles log entry creation, level checking, and enqueuing into the processing pipeline.
package unologger

import (
	"context"
	"os"
	"time"
)

// Debug logs a message at DEBUG level with the given context.
func (l *Logger) Debug(ctx context.Context, format string, args ...interface{}) {
	l.log(ctx, DEBUG, format, args...)
}

// Info logs a message at INFO level with the given context.
func (l *Logger) Info(ctx context.Context, format string, args ...interface{}) {
	l.log(ctx, INFO, format, args...)
}

// Warn logs a message at WARN level with the given context.
func (l *Logger) Warn(ctx context.Context, format string, args ...interface{}) {
	l.log(ctx, WARN, format, args...)
}

// Error logs a message at ERROR level with the given context.
func (l *Logger) Error(ctx context.Context, format string, args ...interface{}) {
	l.log(ctx, ERROR, format, args...)
}

// Fatal logs a message at FATAL level, attempts to close the logger within 2 seconds,
// and then exits the process with status 1.
func (l *Logger) Fatal(ctx context.Context, format string, args ...interface{}) {
	l.log(ctx, FATAL, format, args...)
	_ = CloseDetached(l, 2*time.Second) // Assuming CloseDetached exists and is exported
	os.Exit(1)
}

// WithContext returns a new LoggerWithCtx instance bound to the provided context.
func (l *Logger) WithContext(ctx context.Context) LoggerWithCtx {
	return LoggerWithCtx{l: l, ctx: ctx}
}

// GlobalLogger returns the global logger instance, initializing it with default settings
// (INFO level, UTC timezone) if it hasn't been initialized yet.
func GlobalLogger() *Logger {
	ensureInit()
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalLogger
}

// log is an internal method that creates a log entry, checks its level,
// and enqueues it into the logger's processing pipeline.
// It is called by all public logging methods (Debug, Info, etc.).
func (l *Logger) log(ctx context.Context, level Level, format string, args ...interface{}) {
	// Check if the log level is sufficient for the logger's minimum level.
	if level < Level(l.minLevel.Load()) {
		return
	}

	// Get a log entry from the pool to reduce allocations.
	entry := poolEntry.Get().(*logEntry)
	entry.lvl = level
	entry.ctx = ctx
	entry.t = time.Now()
	entry.tmpl = format
	entry.args = args
	// entry.fields will be populated by context_api or other mechanisms if used.

	// Enqueue the log entry into the processing pipeline.
	l.enqueue(entry) // Assuming l.enqueue exists as an internal method.
}
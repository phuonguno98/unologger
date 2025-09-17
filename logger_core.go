// Copyright (c) 2025 Nguyễn Thanh Phương
// This source code is licensed under the MIT License found in the LICENSE file.

// Package unologger provides a flexible and feature-rich logging library for Go applications.
// This file contains the core logging methods for the Logger struct. These methods
// serve as the primary entry point into the logging pipeline, where they perform
// level-checking, create log entries, and enqueue them for asynchronous processing.

package unologger

import (
	"context"
	"os"
	"time"
)

// Debug logs a message at the DEBUG level.
// The message is only processed if the logger's level is set to DEBUG.
func (l *Logger) Debug(ctx context.Context, format string, args ...interface{}) {
	l.log(ctx, DEBUG, format, args...)
}

// Info logs a message at the INFO level.
func (l *Logger) Info(ctx context.Context, format string, args ...interface{}) {
	l.log(ctx, INFO, format, args...)
}

// Warn logs a message at the WARN level.
func (l *Logger) Warn(ctx context.Context, format string, args ...interface{}) {
	l.log(ctx, WARN, format, args...)
}

// Error logs a message at the ERROR level.
func (l *Logger) Error(ctx context.Context, format string, args ...interface{}) {
	l.log(ctx, ERROR, format, args...)
}

// Fatal logs a message at the FATAL level, attempts to flush all buffered logs,
// and then terminates the application with a call to os.Exit(1).
func (l *Logger) Fatal(ctx context.Context, format string, args ...interface{}) {
	l.log(ctx, FATAL, format, args...)
	// Attempt a graceful shutdown of this logger instance before exiting.
	_ = CloseDetached(l, 2*time.Second)
	os.Exit(1)
}

// WithContext returns a new LoggerWithCtx, which is a lightweight wrapper that
// binds the logger to a specific context. This is useful for creating context-aware
// loggers that can be passed through application layers.
func (l *Logger) WithContext(ctx context.Context) LoggerWithCtx {
	return LoggerWithCtx{l: l, ctx: ctx}
}

// GlobalLogger returns the shared global logger instance.
// If the global logger has not been initialized yet, it will be initialized
// on first use with default settings (INFO level, UTC timezone). This allows
// for "zero-configuration" logging. For custom configurations, call
// InitLoggerWithConfig at application startup.
func GlobalLogger() *Logger {
	ensureInit()
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalLogger
}

// log is the central, internal logging method. It is responsible for:
//  1. Performing a fast, atomic check against the minimum log level.
//  2. Acquiring a reusable logEntry object from a sync.Pool to reduce allocations.
//  3. Populating the logEntry with the current time, context, and message details.
//  4. Passing the populated entry to the enqueue method for asynchronous processing.
func (l *Logger) log(ctx context.Context, level Level, format string, args ...interface{}) {
	// Atomically check if the log level is high enough. This is a fast path
	// to discard logs without the overhead of creating a log entry.
	if level < Level(l.minLevel.Load()) {
		return
	}

	// Acquire a log entry from the pool.
	entry := poolEntry.Get().(*logEntry)
	entry.lvl = level
	entry.ctx = ctx
	entry.t = time.Now()
	entry.tmpl = format
	entry.args = args
	// Note: entry.fields is not populated here. It's extracted from the context
	// later in the pipeline during formatting.

	// Hand off the entry to the asynchronous processing pipeline.
	l.enqueue(entry)
}

// Copyright (c) 2025 Nguyễn Thanh Phương
// This source code is licensed under the MIT License found in the LICENSE file.

// Package unologger provides a flexible and feature-rich logging library for Go applications.
// This file implements the logic for writing formatted log entries to their final
// destinations. It manages multiple writers, handles I/O errors with a configurable
// retry mechanism, and tracks per-writer error statistics.

package unologger

import (
	"io"
	"math/rand"
	"time"
)

// writeToAll is the central dispatch function for writing a formatted log entry.
// It writes the log bytes to all configured destinations.
//
// The routing logic is as follows:
//  1. If `isError` is true (for ERROR and FATAL levels), the log is sent to the `stderr` writer.
//  2. Otherwise, it is sent to the `stdout` writer.
//  3. The log is then sent to the rotation writer (if enabled).
//  4. Finally, the log is sent to all additional `extra` writers.
//
// This function is concurrency-safe. It snapshots the writer configuration under a
// read lock before performing I/O to avoid holding the lock during potentially
// slow write operations.
func (l *Logger) writeToAll(p []byte, isError bool) {
	// Snapshot the writer configuration to avoid holding a lock during I/O.
	l.outputsMu.RLock()
	std := l.stdOut
	errw := l.errOut
	rotSink := l.rotationSink
	extras := make([]writerSink, len(l.extraW))
	copy(extras, l.extraW)
	l.outputsMu.RUnlock()

	// Write to the primary destination (stdout or stderr).
	if isError {
		l.tryWrite("stderr", errw, p)
	} else {
		l.tryWrite("stdout", std, p)
	}

	// Write to the rotation file sink.
	if rotSink != nil {
		l.tryWrite(rotSink.Name, rotSink.Writer, p)
	}

	// Write to all additional writers.
	for _, sink := range extras {
		l.tryWrite(sink.Name, sink.Writer, p)
	}
}

// tryWrite attempts to write a byte slice to a single io.Writer, applying a
// retry policy in case of failure. The `name` parameter is used to track
// error statistics for this specific writer.
func (l *Logger) tryWrite(name string, w io.Writer, p []byte) {
	if w == nil {
		return
	}

	// Snapshot the retry policy.
	l.dynConfig.mu.RLock()
	rp := l.retryPolicy
	l.dynConfig.mu.RUnlock()

	maxRetries := rp.MaxRetries
	if maxRetries < 0 {
		maxRetries = 0
	}

	var err error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		_, err = w.Write(p)
		if err == nil {
			// Write was successful.
			return
		}

		// Write failed; record the error.
		l.writeErrCount.Add(1)
		l.incWriterErr(name)

		if attempt == maxRetries {
			// All retries have been exhausted.
			return
		}

		// Calculate backoff duration for the next retry.
		delay := rp.Backoff
		if rp.Exponential {
			delay *= time.Duration(1 << attempt)
		}
		if rp.Jitter > 0 {
			// Add random jitter to prevent thundering herd.
			delay += time.Duration(rand.Int63n(int64(rp.Jitter)))
		}

		time.Sleep(delay)
	}
}

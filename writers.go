// Copyright (c) 2025 Nguyễn Thanh Phương
// This source code is licensed under the MIT License found in the LICENSE file.

// Package unologger provides a flexible and feature-rich logging library for Go applications.
// This file implements the core logic for writing formatted log entries to various output destinations,
// including standard output, standard error, rotation files, and additional custom writers.
// It incorporates retry/backoff mechanisms and per-writer error statistics to ensure reliable delivery.
package unologger

import (
	"io"
	"time"
)

// writeToAll writes the provided byte slice (formatted log entry) to all configured output writers.
// It directs the log to stdout or stderr based on the `isError` flag, and also to the rotation
// writer (if enabled) and any additional extra writers.
// This method is concurrency-safe as it snapshots the writer configurations before performing I/O.
func (l *Logger) writeToAll(p []byte, isError bool) {
	// Acquire read lock to safely snapshot output configurations.
	l.outputsMu.RLock()
	std := l.stdOut
	errw := l.errOut
	var rotName string
	var rotWriter io.Writer
	if l.rotationSink != nil && l.rotationSink.Writer != nil {
		rotName = l.rotationSink.Name
		rotWriter = l.rotationSink.Writer
	}
	// Create a copy of extra writers to avoid holding the lock during I/O.
	extras := make([]writerSink, len(l.extraW))
	copy(extras, l.extraW)
	l.outputsMu.RUnlock() // Release read lock before performing I/O.

	// Write to primary standard output/error.
	if isError {
		l.safeWrite("stderr", errw, p)
	} else {
		l.safeWrite("stdout", std, p)
	}

	// Write to internal rotation writer (if configured).
	if rotWriter != nil {
		l.safeWrite(rotName, rotWriter, p)
	}

	// Write to all additional extra writers.
	for _, sink := range extras {
		l.safeWrite(sink.Name, sink.Writer, p)
	}
}

// safeWrite attempts to write a byte slice to a given io.Writer,
// incorporating retry and exponential backoff mechanisms based on the logger's retry policy.
// It also tracks write errors per writer.
// If a write operation fails, it will retry up to `MaxRetries` times with increasing delays.
func (l *Logger) safeWrite(name string, w io.Writer, p []byte) {
	if w == nil {
		return // Nothing to write to.
	}

	// Safely snapshot the current retry policy.
	l.dynConfig.mu.RLock()
	rp := l.retryPolicy
	l.dynConfig.mu.RUnlock()

	// Clamp retry configuration values to ensure valid behavior.
	maxRetries := rp.MaxRetries
	if maxRetries < 0 {
		maxRetries = 0 // At least one attempt.
	}
	delay := rp.Backoff
	if delay < 0 {
		delay = 0 // No negative delay.
	}

	var err error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		_, err = w.Write(p) // Attempt to write.
		if err == nil {
			return // Write successful, exit.
		}

		// Write failed, record error.
		l.writeErrCount.Add(1) // Increment total write error count.
		l.incWriterErr(name)   // Increment error count for this specific writer.

		if attempt == maxRetries {
			return // Last attempt failed, give up.
		}

		// Calculate sleep duration for retry.
		sleep := delay
		if rp.Exponential {
			// Exponential backoff: delay * 2^attempt.
			sleep = delay * (1 << attempt)
		}
		if rp.Jitter > 0 {
			// Add random jitter to prevent thundering herd problem.
			// Using time.Now().UnixNano() for a simple pseudo-random number.
			n := time.Now().UnixNano()
			if n < 0 {
				n = -n
			}
			j := time.Duration(n % int64(rp.Jitter))
			sleep += j
		}
		time.Sleep(sleep) // Wait before retrying.
	}
}

// Copyright (c) 2025 Nguyễn Thanh Phương
// This source code is licensed under the MIT License found in the LICENSE file.

// Package unologger provides a flexible and feature-rich logging library for Go applications.
// This file offers functionalities for retrieving logger statistics and for gracefully
// closing logger instances (both global and detached).

package unologger

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// Stats returns comprehensive statistics for the global logger.
// It includes counts for dropped, written, and batched log entries,
// as well as errors encountered during writing and hook execution.
// It also provides the current queue length, detailed writer errors, and a log of hook errors.
func Stats() (dropped, written, batches, writeErrs, hookErrs int64, queueLen int, writerErrs map[string]int64, hookErrLog []HookError) {
	globalMu.RLock()
	l := globalLogger
	globalMu.RUnlock()
	if l == nil {
		// Return zero values if the global logger is not initialized.
		return 0, 0, 0, 0, 0, 0, nil, nil
	}
	// Retrieve statistics from the global logger instance.
	return l.droppedCount.Load(),
		l.writtenCount.Load(),
		l.batchCount.Load(),
		l.writeErrCount.Load(),
		l.hookErrCount.Load(),
		len(l.ch), // Current number of entries in the main processing channel.
		l.getWriterErrorStats(),
		l.GetHookErrors() // Use the public GetHookErrors method.
}

// StatsDetached returns comprehensive statistics for a specific detached logger instance.
// It provides the same metrics as Stats() but for a non-global logger.
func StatsDetached(l *Logger) (dropped, written, batches, writeErrs, hookErrs int64, queueLen int, writerErrs map[string]int64, hookErrLog []HookError) {
	if l == nil {
		// Return zero values if the provided logger is nil.
		return 0, 0, 0, 0, 0, 0, nil, nil
	}
	// Retrieve statistics from the provided logger instance.
	return l.droppedCount.Load(),
		l.writtenCount.Load(),
		l.batchCount.Load(),
		l.writeErrCount.Load(),
		l.hookErrCount.Load(),
		len(l.ch), // Current number of entries in the main processing channel.
		l.getWriterErrorStats(),
		l.GetHookErrors() // Use the public GetHookErrors method.
}

// Close gracefully shuts down the global logger.
// It stops accepting new log entries, waits for all pending log entries to be processed,
// stops hook workers, and closes all associated writers.
// The `timeout` parameter specifies the maximum time to wait for all operations to complete.
// If `timeout` is 0 or less, it waits indefinitely.
// This function is idempotent; calling it multiple times will not cause errors.
// It returns an error if the shutdown process times out.
func Close(timeout time.Duration) error {
	globalMu.RLock()
	l := globalLogger
	globalMu.RUnlock()
	if l == nil || l.closed.IsTrue() {
		// If logger is nil or already closed, return immediately.
		return nil
	}
	return closeLogger(l, timeout) // Delegate to the common closeLogger function.
}

// CloseDetached gracefully shuts down a specific detached logger instance.
// It follows the same shutdown procedure as Close() but applies only to the provided logger.
// It returns an error if the shutdown process times out.
func CloseDetached(l *Logger, timeout time.Duration) error {
	if l == nil || l.closed.IsTrue() {
		// If logger is nil or already closed, return immediately.
		return nil
	}
	return closeLogger(l, timeout) // Delegate to the common closeLogger function.
}

// closeLogger performs the common shutdown logic for both global and detached loggers.
// It ensures that the logger is closed only once.
func closeLogger(l *Logger, timeout time.Duration) error {
	// Use TrySetTrue to ensure the close operation is performed only once.
	if !l.closed.TrySetTrue() {
		return nil // Already closed or another goroutine is closing it.
	}

	close(l.ch) // Close the main log entry channel to signal workers to stop accepting new entries.

	done := make(chan struct{}) // Channel to signal when all workers have finished.
	go func() {
		l.wg.Wait() // Wait for all log processing worker goroutines to complete.
		close(done) // Signal completion.
	}()

	if timeout <= 0 {
		// Wait indefinitely for workers to finish.
		<-done
		// Workers have stopped, now close hook runner and all writers.
		l.closeHookRunner()
		l.closeAllWriters()
		// Print any accumulated writer errors to stderr.
		statsStr := l.formatWriterErrorStats()
		if statsStr != "no writer errors" {
			_, _ = fmt.Fprintln(os.Stderr, statsStr)
		}
		return nil
	}

	// Wait for workers to finish or timeout.
	select {
	case <-done:
		// Workers finished within the timeout.
		l.closeHookRunner()
		l.closeAllWriters()
		statsStr := l.formatWriterErrorStats()
		if statsStr != "no writer errors" {
			_, _ = fmt.Fprintln(os.Stderr, statsStr)
		}
		return nil
	case <-time.After(timeout):
		// Timeout occurred.
		return fmt.Errorf("logger: close timeout after %s", timeout)
	}
}

// incWriterErr increments the error count for a specific writer.
// This is an internal helper function.
func (l *Logger) incWriterErr(name string) {
	val, _ := l.writerErrs.LoadOrStore(name, int64(0))
	l.writerErrs.Store(name, val.(int64)+1)
}

// getWriterErrorStats returns a map of writer names to their error counts.
// This is an internal helper function for statistics.
func (l *Logger) getWriterErrorStats() map[string]int64 {
	stats := make(map[string]int64)
	l.writerErrs.Range(func(key, value any) bool {
		stats[key.(string)] = value.(int64)
		return true
	})
	return stats
}

// formatWriterErrorStats formats the writer error statistics into a human-readable string.
// This is an internal helper function.
func (l *Logger) formatWriterErrorStats() string {
	stats := l.getWriterErrorStats()
	if len(stats) == 0 {
		return "no writer errors"
	}
	var sb strings.Builder
	sb.WriteString("unologger: writer errors: ")
	first := true
	for name, count := range stats {
		if !first {
			sb.WriteString(", ")
		}
		sb.WriteString(fmt.Sprintf("%s=%d", name, count))
		first = false
	}
	return sb.String()
}

// closeAllWriters closes all registered writers (stdout, stderr, extra, rotation).
// This is an internal helper function called during logger shutdown.
func (l *Logger) closeAllWriters() {
	l.outputsMu.Lock() // Acquire lock to safely iterate and close writers.
	defer l.outputsMu.Unlock()

	// Close standard output if it's a Closer (e.g., a file).
	if closer, ok := l.stdOut.(io.Closer); ok {
		if err := closer.Close(); err != nil {
			l.incWriterErr("stdout")
		}
	}
	// Close standard error if it's a Closer.
	if closer, ok := l.errOut.(io.Closer); ok {
		if err := closer.Close(); err != nil {
			l.incWriterErr("stderr")
		}
	}

	// Close extra writers.
	for _, s := range l.extraW {
		if s.Closer != nil {
			if err := s.Closer.Close(); err != nil {
				l.incWriterErr(s.Name)
			}
		}
	}
	l.extraW = nil // Clear the slice after closing.

	// Close rotation writer.
	if l.rotationSink != nil && l.rotationSink.Closer != nil {
		if err := l.rotationSink.Closer.Close(); err != nil {
			l.incWriterErr(l.rotationSink.Name)
		}
	}
	l.rotationSink = nil // Clear the rotation sink.
}

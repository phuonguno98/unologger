// Copyright (c) 2025 Nguyễn Thanh Phương
// This source code is licensed under the MIT License found in the LICENSE file.

// Package unologger provides a flexible and feature-rich logging library for Go applications.
// This file contains functions for gracefully shutting down a logger instance and for
// retrieving detailed runtime statistics, which are essential for monitoring the
// logger's health and performance.

package unologger

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// Stats returns a snapshot of the current performance and error statistics for the global logger.
// It is safe for concurrent use.
//
// Returned values:
//   - dropped: Total number of log entries dropped because the queue was full (in non-blocking mode).
//   - written: Total number of log entries successfully passed to the formatter.
//   - batches: Total number of batches processed by the workers.
//   - writeErrs: Total number of errors encountered when writing to any output.
//   - hookErrs: Total number of errors or panics encountered during hook execution.
//   - queueLen: The number of log entries currently waiting in the processing queue.
//   - writerErrs: A map of writer names to their individual error counts.
//   - hookErrLog: A slice containing recent hook errors (up to a configured maximum).
func Stats() (dropped, written, batches, writeErrs, hookErrs int64, queueLen int, writerErrs map[string]int64, hookErrLog []HookError) {
	l := GlobalLogger() // This ensures the logger is initialized.
	if l == nil {
		return 0, 0, 0, 0, 0, 0, nil, nil
	}
	return StatsDetached(l)
}

// StatsDetached returns a snapshot of the current performance and error statistics for a specific logger instance.
// See the documentation for `Stats()` for a description of the returned values.
func StatsDetached(l *Logger) (dropped, written, batches, writeErrs, hookErrs int64, queueLen int, writerErrs map[string]int64, hookErrLog []HookError) {
	if l == nil {
		return 0, 0, 0, 0, 0, 0, nil, nil
	}
	return l.droppedCount.Load(),
		l.writtenCount.Load(),
		l.batchCount.Load(),
		l.writeErrCount.Load(),
		l.hookErrCount.Load(),
		len(l.ch),
		l.getWriterErrorStats(),
		l.GetHookErrors()
}

// Close gracefully shuts down the global logger, ensuring all buffered logs are written.
// It's crucial to call this at application exit to prevent log loss.
//
// The process is:
//  1. Stop accepting new log entries.
//  2. Wait for all worker goroutines to finish processing the queue.
//  3. Close all output writers.
//
// The timeout parameter specifies the maximum time to wait for this process.
// This function is idempotent; it is safe to call multiple times.
func Close(timeout time.Duration) error {
	l := GlobalLogger()
	if l == nil || l.closed.Load() {
		return nil
	}
	return closeLogger(l, timeout)
}

// CloseDetached gracefully shuts down a specific logger instance.
// See the documentation for `Close()` for details on the shutdown process.
func CloseDetached(l *Logger, timeout time.Duration) error {
	if l == nil || l.closed.Load() {
		return nil
	}
	return closeLogger(l, timeout)
}

// closeLogger contains the core shutdown logic for any logger instance.
func closeLogger(l *Logger, timeout time.Duration) error {
	// Atomically set the `closed` flag. If it was already true, another goroutine
	// is already handling the shutdown, so we can return.
	if !l.closed.TrySetTrue() {
		return nil
	}

	// Close the main channel. This signals the worker loops to stop accepting
	// new entries and to exit once they have processed all remaining entries.
	close(l.ch)

	done := make(chan struct{})
	go func() {
		// Wait for all worker goroutines to finish their work.
		l.wg.Wait()
		// After workers are done, we can safely close the hooks and writers.
		l.closeHookRunner()
		l.closeAllWriters()
		close(done)
	}()

	if timeout <= 0 {
		// Wait indefinitely for shutdown to complete.
		<-done
		l.printFinalStats(os.Stderr)
		return nil
	}

	// Wait for shutdown to complete or for the timeout to expire.
	select {
	case <-done:
		// Shutdown completed successfully within the timeout.
		l.printFinalStats(os.Stderr)
		return nil
	case <-time.After(timeout):
		// Timeout expired before shutdown could complete.
		return fmt.Errorf("unologger: close timed out after %s", timeout)
	}
}

// incWriterErr is a thread-safe method to increment the error count for a specific writer.
func (l *Logger) incWriterErr(name string) {
	// Use an atomic counter per writer to avoid lost updates under contention.
	// Store *atomicI64 in the map and increment atomically.
	if c, ok := l.writerErrs.Load(name); ok {
		switch v := c.(type) {
		case *atomicI64:
			v.Add(1)
			return
		case int64:
			// Backward compatibility in case an int64 was stored previously.
			// Replace with an atomic counter initialized to v+1.
			ai := &atomicI64{}
			ai.Store(v + 1)
			l.writerErrs.Store(name, ai)
			return
		}
	}
	// Not present: create a new atomic counter starting at 1.
	ai := &atomicI64{}
	ai.Store(1)
	if prev, loaded := l.writerErrs.LoadOrStore(name, ai); loaded {
		// Another goroutine beat us; increment that one.
		if p, ok := prev.(*atomicI64); ok {
			p.Add(1)
		} else if iv, ok := prev.(int64); ok {
			tmp := &atomicI64{}
			tmp.Store(iv + 1)
			l.writerErrs.Store(name, tmp)
		}
	}
}

// getWriterErrorStats safely retrieves a snapshot of the writer error counts.
func (l *Logger) getWriterErrorStats() map[string]int64 {
	stats := make(map[string]int64)
	l.writerErrs.Range(func(key, value any) bool {
		name := key.(string)
		switch v := value.(type) {
		case *atomicI64:
			stats[name] = v.Load()
		case int64:
			// Backward compatibility: accept raw int64 values.
			stats[name] = v
		default:
			// Unknown type; ignore.
		}
		return true
	})
	return stats
}

// formatWriterErrorStats creates a summary string of writer errors.
func (l *Logger) formatWriterErrorStats() string {
	stats := l.getWriterErrorStats()
	if len(stats) == 0 {
		return ""
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

// printFinalStats prints the writer error summary to the given writer if there are any errors.
func (l *Logger) printFinalStats(w io.Writer) {
	if statsStr := l.formatWriterErrorStats(); statsStr != "" {
		fmt.Fprintln(w, statsStr)
	}
}

// closeAllWriters iterates through and closes all configured writers that implement io.Closer.
func (l *Logger) closeAllWriters() {
	l.outputsMu.Lock()
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

	// Close any extra writers.
	for _, s := range l.extraW {
		if s.Closer != nil {
			if err := s.Closer.Close(); err != nil {
				l.incWriterErr(s.Name)
			}
		}
	}
	l.extraW = nil

	// Close the rotation writer.
	if l.rotationSink != nil && l.rotationSink.Closer != nil {
		if err := l.rotationSink.Closer.Close(); err != nil {
			l.incWriterErr(l.rotationSink.Name)
		}
	}
	l.rotationSink = nil
}

// Copyright (c) 2025 Nguyễn Thanh Phương
// This source code is licensed under the MIT License found in the LICENSE file.

// Package unologger provides a flexible and feature-rich logging library for Go applications.
// This file manages the hook system, allowing custom functions to be injected into the
// logging pipeline. Hooks can execute synchronously or asynchronously, support timeouts,
// and are designed to be panic-safe. They apply to all log levels (DEBUG, INFO, WARN, ERROR, FATAL).

package unologger

import (
	"context"
	"fmt"
	"time"
)

// startHookRunner initializes and starts the worker pool for processing hooks
// when the logger is configured for asynchronous hook execution.
// It creates a channel for hook tasks and launches worker goroutines.
func (l *Logger) startHookRunner() {
	l.hooksMu.RLock()
	hasHooks := len(l.hooks) > 0
	l.hooksMu.RUnlock()
	// Only start if async hooks are enabled and there are actual hooks to run.
	if !l.hookAsync || !hasHooks {
		return
	}
	// Initialize the hook queue channel.
	l.hookQueueCh = make(chan hookTask, l.hookQueue)
	// Launch worker goroutines to process tasks from the queue.
	for i := 0; i < l.hookWorkers; i++ {
		l.hookWg.Add(1) // Increment WaitGroup counter for each worker.
		go func() {
			defer l.hookWg.Done() // Decrement WaitGroup counter when worker exits.
			// Process tasks from the channel until it's closed.
			for task := range l.hookQueueCh {
				l.runHooks(task.event)
			}
		}()
	}
}

// enqueueHook adds a HookEvent to the asynchronous hook queue if async mode is enabled.
// If async mode is disabled, it executes the hooks synchronously.
// If the queue is full in async mode and DropOldest is not set, it records an error.
func (l *Logger) enqueueHook(ev HookEvent) {
	l.hooksMu.RLock()
	hasHooks := len(l.hooks) > 0
	l.hooksMu.RUnlock()
	if !hasHooks {
		return // No hooks registered, so nothing to do.
	}

	if l.hookAsync {
		// Attempt to send the task to the queue without blocking.
		select {
		case l.hookQueueCh <- hookTask{event: ev}:
			// Task successfully enqueued.
		default:
			// Queue is full, record an error.
			l.recordHookError(ev, ErrHookQueueFull)
		}
	} else {
		// Execute hooks synchronously.
		l.runHooks(ev)
	}
}

// snapshotHooks returns a copy of the currently registered hook functions.
// This ensures that the slice of hooks can be iterated without holding a lock
// during hook execution, preventing deadlocks if hooks themselves acquire locks.
func (l *Logger) snapshotHooks() []HookFunc {
	l.hooksMu.RLock()
	defer l.hooksMu.RUnlock()
	if len(l.hooks) == 0 {
		return nil
	}
	// Create a new slice and copy hooks to it.
	cp := make([]HookFunc, len(l.hooks))
	copy(cp, l.hooks)
	return cp
}

// runHooks executes all registered hook functions for a given HookEvent.
// Each hook is executed with a timeout (if configured) and is protected against panics.
// Any errors or panics during hook execution are recorded.
func (l *Logger) runHooks(ev HookEvent) {
	hooks := l.snapshotHooks() // Get a snapshot of hooks to avoid race conditions.
	if len(hooks) == 0 {
		return
	}

	for _, hk := range hooks {
		// Use an anonymous function to defer panic recovery for each hook.
		func() {
			defer func() {
				if r := recover(); r != nil {
					// Recover from panic and record it as a hook error.
					l.recordHookError(ev, ErrHookPanic)
				}
			}()

			if l.hookTimeout > 0 {
				// Execute hook with a context timeout.
				ctx, cancel := context.WithTimeout(context.Background(), l.hookTimeout)
				defer cancel() // Ensure context resources are released.

				done := make(chan struct{}) // Channel to signal hook completion.
				var err error
				go func() {
					err = hk(ev) // Execute the hook function.
					close(done)  // Signal completion.
				}()

				select {
				case <-ctx.Done():
					// Hook timed out.
					l.recordHookError(ev, ErrHookTimeout)
				case <-done:
					// Hook completed, check for returned error.
					if err != nil {
						l.recordHookError(ev, err)
					}
				}
			} else {
				// Execute hook without a timeout.
				if err := hk(ev); err != nil {
					l.recordHookError(ev, err)
				}
			}
		}() // End of anonymous function for panic recovery.
	}
}

// recordHookError records a hook execution error, increments the error counter,
// and stores detailed error information in a circular buffer (limited by hookErrMax).
func (l *Logger) recordHookError(ev HookEvent, err error) {
	l.hookErrCount.Add(1) // Increment atomic error counter.
	l.hookErrMu.Lock()    // Protect access to the hook error log slice.
	defer l.hookErrMu.Unlock()

	// Ensure hookErrMax is valid, fallback to default if not.
	if l.hookErrMax <= 0 {
		l.hookErrMax = defaultHookErrMax
	}

	// Implement a circular buffer for hook errors.
	if len(l.hookErrLog) >= l.hookErrMax {
		// Remove the oldest elements to make space for new ones.
		trim := len(l.hookErrLog) - (l.hookErrMax - 1)
		if trim < 1 {
			trim = 1 // Ensure at least one element is trimmed if buffer is full.
		}
		l.hookErrLog = append(l.hookErrLog[trim:], HookError{
			Time:    time.Now(),
			Level:   ev.Level,
			Module:  ev.Module,
			Message: ev.Message,
			Err:     err,
		})
		return
	}
	// Append new error if buffer is not full.
	l.hookErrLog = append(l.hookErrLog, HookError{
		Time:    time.Now(),
		Level:   ev.Level,
		Module:  ev.Module,
		Message: ev.Message,
		Err:     err,
	})
}

// GetHookErrors returns a copy of the recorded hook errors.
// This allows inspection of recent hook failures without direct access to the internal buffer.
func (l *Logger) GetHookErrors() []HookError {
	l.hookErrMu.Lock() // Protect access to the hook error log slice.
	defer l.hookErrMu.Unlock()
	// Return a copy to prevent external modification of the internal slice.
	out := make([]HookError, len(l.hookErrLog))
	copy(out, l.hookErrLog)
	return out
}

// closeHookRunner closes the hook queue channel and waits for all hook workers to finish.
// This is typically called during logger shutdown. The hook runner can be restarted
// after being closed, for example, if hooks are dynamically reconfigured.
func (l *Logger) closeHookRunner() {
	if l.hookAsync && l.hookQueueCh != nil {
		close(l.hookQueueCh) // Close the channel to signal workers to exit.
		l.hookWg.Wait()      // Wait for all worker goroutines to complete.
		l.hookQueueCh = nil  // Reset the channel to allow restarting the runner.
	}
}

// ErrHookQueueFull is returned when a hook event cannot be enqueued because the queue is full.
var ErrHookQueueFull = fmt.Errorf("hook queue full")

// ErrHookTimeout is returned when a hook function exceeds its configured execution timeout.
var ErrHookTimeout = fmt.Errorf("hook timeout")

// ErrHookPanic is returned when a hook function panics during execution.
var ErrHookPanic = fmt.Errorf("hook panic")

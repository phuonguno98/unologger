// Copyright (c) 2025 Nguyễn Thanh Phương
// This source code is licensed under the MIT License found in the LICENSE file.

// Package unologger provides a flexible and feature-rich logging library for Go applications.
// This file implements the hook system, which allows for custom functions to be executed
// as part of the logging pipeline. Hooks provide a powerful way to extend the logger's
// functionality, for example, by sending notifications to external services for
// certain log events.

package unologger

import (
	"context"
	"fmt"
	"time"
)

// startHookRunner starts the worker pool for processing hooks asynchronously.
// This method is called internally when the logger is configured with async hooks
// and there is at least one hook registered.
func (l *Logger) startHookRunner() {
	l.hooksMu.RLock()
	hasHooks := len(l.hooks) > 0
	l.hooksMu.RUnlock()
	if !l.hookAsync || !hasHooks {
		return
	}

	l.hookQueueCh = make(chan hookTask, l.hookQueue)
	for i := 0; i < l.hookWorkers; i++ {
		l.hookWg.Add(1)
		go func() {
			defer l.hookWg.Done()
			for task := range l.hookQueueCh {
				l.runHooks(task.event)
			}
		}()
	}
}

// enqueueHook processes a log event with the registered hooks.
// If async mode is enabled, it adds the event to a non-blocking queue.
// If the queue is full, an error is recorded. If async is disabled,
// it executes the hooks synchronously in the same goroutine.
func (l *Logger) enqueueHook(ev HookEvent) {
	l.hooksMu.RLock()
	hasHooks := len(l.hooks) > 0
	l.hooksMu.RUnlock()
	if !hasHooks {
		return // No-op if no hooks are registered.
	}

	if l.hookAsync {
		select {
		case l.hookQueueCh <- hookTask{event: ev}:
			// Task successfully enqueued.
		default:
			// Queue is full.
			l.recordHookError(ev, ErrHookQueueFull)
		}
	} else {
		// Execute synchronously.
		l.runHooks(ev)
	}
}

// snapshotHooks creates and returns a copy of the current hook functions.
// This is a crucial step to prevent deadlocks. By iterating over a copy,
// we avoid holding a read lock on l.hooksMu while executing the hooks,
// which might themselves try to acquire a lock on the logger.
func (l *Logger) snapshotHooks() []HookFunc {
	l.hooksMu.RLock()
	defer l.hooksMu.RUnlock()
	if len(l.hooks) == 0 {
		return nil
	}
	cp := make([]HookFunc, len(l.hooks))
	copy(cp, l.hooks)
	return cp
}

// runHooks executes all registered hooks for a given event.
// Each hook is executed in a panic-safe manner. If a timeout is configured,
// each hook's execution is constrained by it. Errors and panics are captured
// and recorded.
func (l *Logger) runHooks(ev HookEvent) {
	hooks := l.snapshotHooks()
	if len(hooks) == 0 {
		return
	}

	for _, hk := range hooks {
		// IIFE to scope the defer for panic recovery.
		func() {
			defer func() {
				if r := recover(); r != nil {
					l.recordHookError(ev, fmt.Errorf("%w: %v", ErrHookPanic, r))
				}
			}()

			if l.hookTimeout > 0 {
				l.runHookWithTimeout(hk, ev)
			} else {
				l.runHookWithoutTimeout(hk, ev)
			}
		}()
	}
}

// runHookWithTimeout executes a single hook with a timeout.
func (l *Logger) runHookWithTimeout(hk HookFunc, ev HookEvent) {
	ctx, cancel := context.WithTimeout(context.Background(), l.hookTimeout)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- hk(ev)
	}()

	select {
	case <-ctx.Done():
		l.recordHookError(ev, ErrHookTimeout)
	case err := <-done:
		if err != nil {
			l.recordHookError(ev, err)
		}
	}
}

// runHookWithoutTimeout executes a single hook without a timeout.
func (l *Logger) runHookWithoutTimeout(hk HookFunc, ev HookEvent) {
	if err := hk(ev); err != nil {
		l.recordHookError(ev, err)
	}
}

// recordHookError atomically increments the hook error counter and adds a
// detailed error to a circular buffer, which holds up to hookErrMax entries.
func (l *Logger) recordHookError(ev HookEvent, err error) {
	l.hookErrCount.Add(1)
	l.hookErrMu.Lock()
	defer l.hookErrMu.Unlock()

	if l.hookErrMax <= 0 {
		l.hookErrMax = defaultHookErrMax
	}

	newErr := HookError{
		Time:    time.Now(),
		Level:   ev.Level,
		Module:  ev.Module,
		Message: ev.Message,
		Err:     err,
	}

	if len(l.hookErrLog) >= l.hookErrMax {
		// Evict the oldest error to make room.
		l.hookErrLog = append(l.hookErrLog[1:], newErr)
	} else {
		l.hookErrLog = append(l.hookErrLog, newErr)
	}
}

// GetHookErrors returns a safe copy of the recent hook execution errors.
func (l *Logger) GetHookErrors() []HookError {
	l.hookErrMu.Lock()
	defer l.hookErrMu.Unlock()
	out := make([]HookError, len(l.hookErrLog))
	copy(out, l.hookErrLog)
	return out
}

// closeHookRunner gracefully shuts down the asynchronous hook processing system.
// It closes the queue and waits for all worker goroutines to finish their tasks.
// It resets the queue channel to nil, allowing the runner to be restarted later.
func (l *Logger) closeHookRunner() {
	if l.hookAsync && l.hookQueueCh != nil {
		close(l.hookQueueCh)
		l.hookWg.Wait()
		l.hookQueueCh = nil // Allow runner to be restarted.
	}
}

// ErrHookQueueFull signifies that a log event could not be processed by an
// async hook because the hook queue was full.
var ErrHookQueueFull = fmt.Errorf("hook queue full")

// ErrHookTimeout signifies that a hook function failed to complete within
// its configured timeout.
var ErrHookTimeout = fmt.Errorf("hook timeout")

// ErrHookPanic signifies that a hook function panicked during execution.
// The panic value is captured and included in the recorded error.
var ErrHookPanic = fmt.Errorf("hook panic")

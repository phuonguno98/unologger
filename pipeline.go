// Copyright (c) 2025 Nguyễn Thanh Phương
// This source code is licensed under the MIT License found in the LICENSE file.

// Package unologger provides a flexible and feature-rich logging library for Go applications.
// This file implements the core log processing pipeline, which includes enqueuing,
// batching, formatting, applying hooks, data masking, and writing log entries to various outputs.
// It supports all log levels: DEBUG, INFO, WARN, ERROR, FATAL.

package unologger

import (
	"fmt"
	"os"
	"time"
)

// ctxFieldsKey is the context key used to store custom Fields in the context.
const ctxFieldsKey ctxKey = "fields"

// enqueue adds a logEntry to the logger's internal channel.
// It respects the NonBlocking and DropOldest configuration settings.
// If the logger is closed, the entry is immediately returned to the pool.
func (l *Logger) enqueue(e *logEntry) {
	if l.closed.IsTrue() {
		poolEntry.Put(e) // Return entry to pool if logger is closed.
		return
	}

	if l.nonBlocking {
		// Attempt to send the entry without blocking.
		select {
		case l.ch <- e:
			// Entry successfully enqueued.
		default:
			// Channel is full.
			if l.dropOldest {
				// If dropOldest is enabled, try to remove an old entry to make space.
				select {
				case old := <-l.ch:
					if old != nil {
						l.droppedCount.Add(1) // Increment dropped count.
						poolEntry.Put(old)    // Return old entry to pool.
					}
				default:
					// No old entry to drop immediately, proceed to try enqueuing.
				}
				// Attempt to enqueue again after potentially dropping an old entry.
				select {
				case l.ch <- e:
					// Entry successfully enqueued.
				default:
					// Still full, drop the current entry.
					l.droppedCount.Add(1)
					poolEntry.Put(e)
				}
			} else {
				// Non-blocking and channel full, but not dropping oldest. Drop current entry.
				l.droppedCount.Add(1)
				poolEntry.Put(e)
			}
		}
	} else {
		// Blocking enqueue: wait until space is available in the channel.
		l.ch <- e
	}
}

// workerLoop is the main goroutine for each worker. It consumes log entries from the channel,
// batches them, and processes/writes them.
func (l *Logger) workerLoop() {
	defer l.wg.Done() // Signal completion to the WaitGroup when the worker exits.

	// Get a logBatch from the pool for reuse.
	batch := poolBatch.Get().(*logBatch)
	batch.items = batch.items[:0] // Reset slice length.
	batch.created = time.Now()    // Mark batch creation time.

	// flush is a helper function to process the current batch.
	flush := func() {
		if len(batch.items) > 0 {
			l.processBatch(batch.items)   // Process the collected log entries.
			batch.items = batch.items[:0] // Clear the batch for next use.
			batch.created = time.Now()    // Reset batch creation time.
			l.batchCount.Add(1)           // Increment batch counter.
		}
	}

	// Initial batch wait duration.
	wait := time.Duration(l.batchWaitA.Load())
	if wait <= 0 {
		wait = time.Second // Default to 1 second if invalid.
	}
	timer := time.NewTimer(wait) // Timer for flushing batches based on time.
	defer timer.Stop()           // Ensure timer is stopped when worker exits.

	for {
		select {
		case e, ok := <-l.ch:
			// Received a log entry from the channel.
			if !ok {
				// Channel is closed, indicating logger shutdown. Flush any remaining entries.
				flush()
				poolBatch.Put(batch) // Return batch to pool.
				return               // Exit worker loop.
			}
			batch.items = append(batch.items, e) // Add entry to the current batch.

			// Check if batch size limit is reached.
			size := int(l.batchSizeA.Load())
			if size <= 0 {
				size = 1 // Default to 1 if invalid.
			}
			if len(batch.items) >= size {
				flush() // Flush the batch if size limit is reached.
				// Reset the timer after flushing.
				if !timer.Stop() {
					<-timer.C // Drain the timer channel if it had already fired.
				}
				wait = time.Duration(l.batchWaitA.Load())
				if wait <= 0 {
					wait = time.Second
				}
				timer.Reset(wait)
			}
		case <-timer.C:
			// Timer fired, flush the batch based on time limit.
			flush()
			// Reset the timer for the next interval.
			wait = time.Duration(l.batchWaitA.Load())
			if wait <= 0 {
				wait = time.Second
			}
			timer.Reset(wait)
		}
	}
}

// processBatch processes a slice of logEntry instances.
// It applies context fields, formats messages, applies masking,
// enqueues hooks, formats the log entry, and writes it to the appropriate writers.
func (l *Logger) processBatch(entries []*logEntry) {
	for _, e := range entries {
		l.writtenCount.Add(1) // Increment total written count.

		// Safely read the current timezone location.
		l.locMu.RLock()
		loc := l.loc
		l.locMu.RUnlock()

		// Extract metadata from the log entry's context.
		module, _ := e.ctx.Value(ctxModuleKey).(string)
		traceID, _ := e.ctx.Value(ctxTraceIDKey).(string)
		flowID, _ := e.ctx.Value(ctxFlowIDKey).(string)

		// Merge fields from context and log entry.
		ctxFields, _ := e.ctx.Value(ctxFieldsKey).(Fields)
		mergedFields := make(Fields)
		for k, v := range ctxFields {
			mergedFields[k] = v
		}
		for k, v := range e.fields {
			mergedFields[k] = v
		}

		// Format the log message and apply masking.
		msg := fmt.Sprintf(e.tmpl, e.args...)
		jsonMode := l.jsonFmtFlag.Load()    // Get current JSON format setting.
		msg = l.applyMasking(msg, jsonMode) // Apply masking based on JSON mode.

		// Create a HookEvent for processing by hooks.
		hookEv := HookEvent{
			Time:     e.t.In(loc), // Convert timestamp to logger's timezone.
			Level:    e.lvl,
			Module:   module,
			Message:  msg,
			TraceID:  traceID,
			FlowID:   flowID,
			Attrs:    nil,          // Attrs are no longer directly used in HookEvent, merged into Fields.
			Fields:   mergedFields, // Use the merged fields.
			JSONMode: jsonMode,
		}
		l.enqueueHook(hookEv) // Enqueue the event for hook processing.

		// Format the log entry using the configured formatter.
		b, err := l.formatter.Format(hookEv)
		if err != nil {
			// Handle formatter errors: print to stderr and increment error count.
			_, _ = fmt.Fprintf(os.Stderr, "unologger: formatter error: %v\n", err)
			l.writeErrCount.Add(1)
			continue // Skip writing this entry and proceed to the next.
		}

		// Determine if the log should also go to stderr (for ERROR and FATAL levels).
		isErr := e.lvl >= ERROR
		l.writeToAll(b, isErr) // Write the formatted log to all configured writers.
		l.recycleEntry(e)      // Return the log entry to the pool.
	}
}

// recycleEntry cleans up references within a logEntry before returning it to the sync.Pool.
// This helps prevent memory leaks and ensures proper reuse of pooled objects.
func (l *Logger) recycleEntry(e *logEntry) {
	// Clear large or sensitive references to aid garbage collection.
	e.ctx = nil
	e.args = nil
	e.tmpl = ""
	e.fields = nil // Clear custom fields.
	// Other fields are small value types and don't need explicit clearing.
	poolEntry.Put(e) // Return the entry to the pool.
}

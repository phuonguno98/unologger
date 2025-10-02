// Copyright (c) 2025 Nguyễn Thanh Phương
// This source code is licensed under the MIT License found in the LICENSE file.

// Package unologger provides a flexible and feature-rich logging library for Go applications.
// This file implements the core asynchronous processing pipeline. It contains the logic
// for the worker goroutines that batch, format, and write log entries, forming the
// high-performance engine of the logger.

package unologger

import (
	"fmt"
	"os"
	"time"
)

// enqueue adds a log entry to the logger's processing channel.
// This method contains the logic for both blocking and non-blocking behavior.
//
// Behavior paths:
//
//  1. If the logger is closed, the entry is immediately discarded and recycled.
//
//  2. If in blocking mode (`nonBlocking` is false), it will wait for space in the channel.
//
//  3. If in non-blocking mode (`nonBlocking` is true):
//     a. It first tries to send the entry.
//
//     b. If the channel is full and `dropOldest` is true, it attempts to remove the
//     oldest entry from the channel to make space for the new one.
//
//     c. If the channel is full and `dropOldest` is false (or if making space fails),
//     the new entry is dropped.
func (l *Logger) enqueue(e *logEntry) {
	if l.closed.Load() {
		recycleEntry(e)
		return
	}

	if !l.nonBlocking {
		// Blocking mode: wait for space.
		l.ch <- e
		return
	}

	// Non-blocking mode.
	if l.dropOldest {
		// Try to drop the oldest entry to make room.
		select {
		case l.ch <- e:
			// Enqueued successfully.
		default:
			// Channel is full, try to dequeue the oldest and enqueue the new one.
			select {
			case oldest := <-l.ch:
				// Dropped the oldest entry.
				l.droppedCount.Add(1)
				recycleEntry(oldest)
				// Now try to enqueue the new entry again.
				select {
				case l.ch <- e:
					// Success.
				default:
					// Still full, drop the new entry.
					l.droppedCount.Add(1)
					recycleEntry(e)
				}
			default:
				// Channel is full and couldn't even drop an old one, so drop the new one.
				l.droppedCount.Add(1)
				recycleEntry(e)
			}
		}
	} else {
		// Default non-blocking: drop the new entry if the queue is full.
		select {
		case l.ch <- e:
			// Enqueued successfully.
		default:
			// Channel is full, drop the current entry.
			l.droppedCount.Add(1)
			recycleEntry(e)
		}
	}
}

// workerLoop is the main loop for a single worker goroutine. It is responsible for
// receiving log entries, collecting them into batches, and flushing them for processing.
// Batching is triggered by two conditions: the batch reaching its maximum size, or a
// timeout expiring.
func (l *Logger) workerLoop() {
	defer l.wg.Done()

	batch := poolBatch.Get().(*logBatch)
	defer poolBatch.Put(batch) // Ensure batch is returned to the pool on exit.

	batch.items = batch.items[:0]
	batch.created = time.Now()

	// flush is a closure to process the current batch.
	flush := func() {
		if len(batch.items) > 0 {
			l.processBatch(batch.items)
			l.batchCount.Add(1)
			// Reset batch for the next collection.
			for i := range batch.items {
				batch.items[i] = nil // Avoid memory leaks.
			}
			batch.items = batch.items[:0]
			batch.created = time.Now()
		}
	}

	// The timer triggers a flush when the MaxWait duration is reached.
	wait := time.Duration(l.batchWaitA.Load())
	if wait <= 0 {
		wait = time.Second
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()

	for {
		select {
		case e, ok := <-l.ch:
			if !ok {
				// Channel closed, meaning the logger is shutting down.
				// Flush any remaining entries and exit the worker.
				flush()
				return
			}

			batch.items = append(batch.items, e)

			// Flush if the batch size limit is reached.
			size := int(l.batchSizeA.Load())
			if size <= 0 {
				size = 1
			}
			if len(batch.items) >= size {
				flush()
				// It's crucial to stop and drain the timer before resetting it
				// to prevent race conditions with the timer channel.
				if !timer.Stop() {
					select {
					case <-timer.C: // Drain the channel.
					default:
					}
				}
				timer.Reset(wait)
			}

		case <-timer.C:
			// Timer fired, flush the batch regardless of its size.
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

// processBatch orchestrates the processing of a slice of log entries.
// For each entry, it formats the message, applies masking, triggers hooks,
// formats the final output, and writes it to the configured destinations.
func (l *Logger) processBatch(entries []*logEntry) {
	for _, e := range entries {
		l.writtenCount.Add(1)

		l.locMu.RLock()
		loc := l.loc
		l.locMu.RUnlock()

		// Extract metadata from the context.
		module, _ := e.ctx.Value(ctxModuleKey).(string)
		traceID, _ := e.ctx.Value(ctxTraceIDKey).(string)
		flowID, _ := e.ctx.Value(ctxFlowIDKey).(string)
		ctxFields, _ := e.ctx.Value(ctxFieldsKey).(Fields)

		// Merge fields from context and the log call itself.
		mergedFields := make(Fields, len(ctxFields)+len(e.fields))
		for k, v := range ctxFields {
			mergedFields[k] = v
		}
		for k, v := range e.fields {
			mergedFields[k] = v
		}

		// Format the log message and apply masking.
		msg := fmt.Sprintf(e.tmpl, e.args...)
		jsonMode := l.jsonFmtFlag.Load()
		msg = l.applyMasking(msg, jsonMode)

		// Prepare and enqueue the event for the hook system.
		hookEv := HookEvent{
			Time:     e.t.In(loc),
			Level:    e.lvl,
			Module:   module,
			Message:  msg,
			TraceID:  traceID,
			FlowID:   flowID,
			Attrs:    mergedFields, // Attrs is now an alias for Fields.
			Fields:   mergedFields,
			JSONMode: jsonMode,
		}
		l.enqueueHook(hookEv)

		// Format the final log line.
		l.formatterMu.RLock()    // Acquire read lock
		formatter := l.formatter // Get the current formatter
		l.formatterMu.RUnlock()  // Release read lock

		b, err := formatter.Format(hookEv)
		if err != nil {
			fmt.Fprintf(os.Stderr, "unologger: formatter error: %v\n", err)
			l.writeErrCount.Add(1)
			recycleEntry(e) // Recycle even on format error.
			continue
		}

		// Write to configured outputs. WARN and above go to stderr per documentation.
		isErrLevel := e.lvl >= WARN
		l.writeToAll(b, isErrLevel)
		recycleEntry(e)
	}
}

// recycleEntry resets a logEntry and returns it to the sync.Pool.
// Nil-ing out pointers helps the GC by breaking references.
func recycleEntry(e *logEntry) {
	e.ctx = nil
	e.args = nil
	e.tmpl = ""
	e.fields = nil
	poolEntry.Put(e)
}

// Copyright (c) 2025 Nguyễn Thanh Phương
// This source code is licensed under the MIT License found in the LICENSE file.

// Package unologger - pipeline.go
// Quản lý pipeline xử lý log: enqueue, gom batch, format, gọi hook, masking và ghi ra writer.
// Hỗ trợ đầy đủ các cấp độ log: DEBUG, INFO, WARN, ERROR, FATAL.

package unologger

import (
	"fmt"
	"os"
	"time"
)

const ctxFieldsKey ctxKey = "fields"

// enqueue đưa logEntry vào channel theo cấu hình NonBlocking/DropOldest.
func (l *Logger) enqueue(e *logEntry) {
	if l.closed.IsTrue() {
		poolEntry.Put(e)
		return
	}
	if l.nonBlocking {
		select {
		case l.ch <- e:
		default:
			if l.dropOldest {
				// NEW: lấy ra một entry cũ (nếu có), tăng droppedCount và trả về pool
				select {
				case old := <-l.ch:
					if old != nil {
						l.droppedCount.Add(1)
						poolEntry.Put(old)
					}
				default:
				}
				select {
				case l.ch <- e:
				default:
					l.droppedCount.Add(1)
					poolEntry.Put(e)
				}
			} else {
				l.droppedCount.Add(1)
				poolEntry.Put(e)
			}
		}
	} else {
		l.ch <- e
	}
}

// workerLoop tiêu thụ channel log và gom batch để xử lý/ghi.
func (l *Logger) workerLoop() {
	defer l.wg.Done()

	batch := poolBatch.Get().(*logBatch)
	batch.items = batch.items[:0]
	batch.created = time.Now()

	flush := func() {
		if len(batch.items) > 0 {
			l.processBatch(batch.items)
			batch.items = batch.items[:0]
			batch.created = time.Now()
			l.batchCount.Add(1)
		}
	}

	// Đọc batchWait lần đầu
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
				// Flush ngay khi shutdown
				flush()
				poolBatch.Put(batch)
				return
			}
			batch.items = append(batch.items, e)

			size := int(l.batchSizeA.Load())
			if size <= 0 {
				size = 1
			}
			if len(batch.items) >= size {
				flush()
				if !timer.Stop() {
					<-timer.C
				}
				wait = time.Duration(l.batchWaitA.Load())
				if wait <= 0 {
					wait = time.Second
				}
				timer.Reset(wait)
			}
		case <-timer.C:
			flush()
			wait = time.Duration(l.batchWaitA.Load())
			if wait <= 0 {
				wait = time.Second
			}
			timer.Reset(wait)
		}
	}
}

// processBatch xử lý một batch logEntry, hỗ trợ JSON/text và route stderr cho lỗi.
func (l *Logger) processBatch(entries []*logEntry) {
	for _, e := range entries {
		l.writtenCount.Add(1)

		// Đọc location an toàn
		l.locMu.RLock()
		loc := l.loc
		l.locMu.RUnlock()

		module, _ := e.ctx.Value(ctxModuleKey).(string)
		traceID, _ := e.ctx.Value(ctxTraceIDKey).(string)
		flowID, _ := e.ctx.Value(ctxFlowIDKey).(string)

		// Lấy fields từ context và merge với fields của entry
		ctxFields, _ := e.ctx.Value(ctxFieldsKey).(Fields)
		mergedFields := make(Fields)
		for k, v := range ctxFields {
			mergedFields[k] = v
		}
		for k, v := range e.fields {
			mergedFields[k] = v
		}

		msg := fmt.Sprintf(e.tmpl, e.args...)
		msg = l.applyMasking(msg, l.jsonFmtFlag.Load()) // Apply masking based on JSON mode

		hookEv := HookEvent{
			Time:     e.t.In(loc),
			Level:    e.lvl,
			Module:   module,
			Message:  msg,
			TraceID:  traceID,
			FlowID:   flowID,
			Attrs:    nil,          // Attrs không còn được sử dụng trực tiếp
			Fields:   mergedFields, // Sử dụng mergedFields
			JSONMode: l.jsonFmtFlag.Load(),
		}
		l.enqueueHook(hookEv)

		// Format log entry
		b, err := l.formatter.Format(hookEv)
		if err != nil {
			// Handle formatter error, maybe log to stderr directly
			_, _ = fmt.Fprintf(os.Stderr, "unologger: formatter error: %v\n", err)
			l.writeErrCount.Add(1)
			// Continue to next entry or return
			continue
		}

		// ERROR và FATAL đều ghi ra stderr
		isErr := e.lvl >= ERROR
		l.writeToAll(b, isErr)
		l.recycleEntry(e)
	}
}

// recycleEntry làm sạch tham chiếu trong logEntry trước khi trả về pool.
func (l *Logger) recycleEntry(e *logEntry) {
	// xóa tham chiếu lớn/nhạy cảm trước khi Put
	e.ctx = nil
	e.args = nil
	e.tmpl = ""
	e.fields = nil // NEW: Xóa fields
	// các trường còn lại là kiểu giá trị nhỏ
	poolEntry.Put(e)
}

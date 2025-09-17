// Copyright (c) 2025 Nguyễn Thanh Phương
// This source code is licensed under the MIT License found in the LICENSE file.

// Package unologger - pipeline.go
// Quản lý pipeline xử lý log: enqueue, gom batch, format, gọi hook, masking và ghi ra writer.
// Hỗ trợ đầy đủ các cấp độ log: DEBUG, INFO, WARN, ERROR, FATAL.

package unologger

import (
	"encoding/json"
	"fmt"
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
	if l.jsonFmtFlag.Load() { // NEW: dùng cờ atomic
		// Encode từng entry và tách theo isError để route đúng stderr/stdout
		bufStd := make([][]byte, 0, len(entries))
		bufErr := make([][]byte, 0, len(entries))
		for _, e := range entries {
			b := l.formatJSONEntry(e)
			if e.lvl >= ERROR {
				bufErr = append(bufErr, b)
			} else {
				bufStd = append(bufStd, b)
			}
			l.recycleEntry(e)
		}

		if len(bufStd) > 0 {
			total := 0
			for _, b := range bufStd {
				total += len(b)
			}
			out := make([]byte, 0, total)
			for _, b := range bufStd {
				out = append(out, b...)
			}
			l.writeToAll(out, false)
		}

		if len(bufErr) > 0 {
			total := 0
			for _, b := range bufErr {
				total += len(b)
			}
			out := make([]byte, 0, total)
			for _, b := range bufErr {
				out = append(out, b...)
			}
			l.writeToAll(out, true)
		}
	} else {
		for _, e := range entries {
			l.processTextEntry(e)
			l.recycleEntry(e)
		}
	}
}

// processTextEntry format, mask, gửi hook và ghi ra output text.
func (l *Logger) processTextEntry(e *logEntry) {
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
	msg = l.applyMasking(msg, false)

	hookEv := HookEvent{
		Time:     e.t.In(loc),
		Level:    e.lvl,
		Module:   module,
		Message:  msg,
		TraceID:  traceID,
		FlowID:   flowID,
		Attrs:    nil,          // Attrs không còn được sử dụng trực tiếp
		Fields:   mergedFields, // Sử dụng mergedFields
		JSONMode: false,
	}
	l.enqueueHook(hookEv)

	// ERROR và FATAL đều ghi ra stderr
	isErr := e.lvl >= ERROR
	ts := e.t.In(loc).Format("2006-01-02 15:04:05.000 MST")
	meta := ""
	if l.enableOTEL.Load() {
		if tsid := formatTraceSpanID(e.ctx); tsid != "" {
			meta += fmt.Sprintf(" trace/span=%s", tsid)
		}
	} else if traceID != "" {
		meta += fmt.Sprintf(" trace=%s", traceID)
	}
	if flowID != "" {
		meta += fmt.Sprintf(" flow=%s", flowID)
	}
	if len(mergedFields) > 0 {
		meta += fmt.Sprintf(" fields=%v", mergedFields)
	}

	line := fmt.Sprintf("%s [%s] (%s)%s %s\n", ts, e.lvl.String(), module, meta, msg)

	l.writeToAll([]byte(line), isErr)
}

// formatJSONEntry format, mask, gửi hook và trả về JSON-encoded bytes (có newline).
func (l *Logger) formatJSONEntry(e *logEntry) []byte {
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
	msg = l.applyMasking(msg, true)

	hookEv := HookEvent{
		Time:     e.t.In(loc),
		Level:    e.lvl,
		Module:   module,
		Message:  msg,
		TraceID:  traceID,
		FlowID:   flowID,
		Attrs:    nil,          // Attrs không còn được sử dụng trực tiếp
		Fields:   mergedFields, // Sử dụng mergedFields
		JSONMode: true,
	}
	l.enqueueHook(hookEv)

	// Struct giữ thứ tự field như khai báo
	type jsonEntry struct {
		Time      string            `json:"time"`
		Level     string            `json:"level"`
		Module    string            `json:"module,omitempty"`
		TraceSpan string            `json:"trace_span,omitempty"`
		TraceID   string            `json:"trace_id,omitempty"`
		FlowID    string            `json:"flow_id,omitempty"`
		Attrs     map[string]string `json:"attrs,omitempty"`
		Message   string            `json:"message"`
		Fields    Fields            `json:"fields,omitempty"` // NEW: Thêm trường fields
	}

	entry := jsonEntry{
		Time:    e.t.In(loc).Format(time.RFC3339Nano),
		Level:   e.lvl.String(),
		Module:  module,
		Message: msg,
		Fields:  mergedFields, // NEW: Gán mergedFields
	}

	if l.enableOTEL.Load() {
		if tsid := formatTraceSpanID(e.ctx); tsid != "" {
			entry.TraceSpan = tsid
		}
	} else if traceID != "" {
		entry.TraceID = traceID
	}

	if flowID != "" {
		entry.FlowID = flowID
	}

	b, _ := json.Marshal(entry)
	b = append(b, '\n')
	return b
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

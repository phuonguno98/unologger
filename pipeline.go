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
				select {
				case <-l.ch:
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

// workerLoop đọc từ channel và gom batch để xử lý.
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

	timer := time.NewTimer(l.batchWait)
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
			if len(batch.items) >= l.batchSize {
				flush()
				if !timer.Stop() {
					<-timer.C
				}
				timer.Reset(l.batchWait)
			}
		case <-timer.C:
			flush()
			timer.Reset(l.batchWait)
		}
	}
}

// processBatch xử lý một batch logEntry.
func (l *Logger) processBatch(entries []*logEntry) {
	if l.jsonFmt {
		// Tối ưu batch JSON: encode từng log và ghi một lần
		buf := make([][]byte, 0, len(entries))
		for _, e := range entries {
			buf = append(buf, l.formatJSONEntry(e))
			poolEntry.Put(e)
		}
		// Ghép tất cả vào một slice byte duy nhất
		totalLen := 0
		for _, b := range buf {
			totalLen += len(b)
		}
		out := make([]byte, 0, totalLen)
		for _, b := range buf {
			out = append(out, b...)
		}
		// Với JSON, không phân biệt error writer
		l.writeToAll(out, false)
	} else {
		for _, e := range entries {
			l.processTextEntry(e)
			poolEntry.Put(e)
		}
	}
}

// processTextEntry format, mask, gọi hooks, và ghi ra output text.
func (l *Logger) processTextEntry(e *logEntry) {
	l.writtenCount.Add(1)

	module, _ := e.ctx.Value(ctxModuleKey).(string)
	traceID, _ := e.ctx.Value(ctxTraceIDKey).(string)
	flowID, _ := e.ctx.Value(ctxFlowIDKey).(string)
	attrs, _ := e.ctx.Value(ctxAttrsKey).(map[string]string)

	msg := fmt.Sprintf(e.tmpl, e.args...)
	msg = l.applyMasking(msg, false)

	hookEv := HookEvent{
		Time:     e.t.In(l.loc),
		Level:    e.lvl,
		Module:   module,
		Message:  msg,
		TraceID:  traceID,
		FlowID:   flowID,
		Attrs:    attrs,
		JSONMode: false,
	}
	l.enqueueHook(hookEv)

	// ERROR và FATAL đều ghi ra stderr
	isErr := e.lvl >= ERROR
	ts := e.t.In(l.loc).Format("2006-01-02 15:04:05.000 MST")
	meta := ""
	if l.enableOTEL {
		if tsid := formatTraceSpanID(e.ctx); tsid != "" {
			meta += fmt.Sprintf(" trace/span=%s", tsid)
		}
	} else if traceID != "" {
		meta += fmt.Sprintf(" trace=%s", traceID)
	}
	if flowID != "" {
		meta += fmt.Sprintf(" flow=%s", flowID)
	}
	if len(attrs) > 0 {
		meta += fmt.Sprintf(" attrs=%v", attrs)
	}

	line := fmt.Sprintf("%s [%s] (%s)%s %s\n", ts, e.lvl.String(), module, meta, msg)
	l.writeToAll([]byte(line), isErr)
}

// formatJSONEntry format, mask, gọi hooks, và trả về []byte JSON đã encode.
func (l *Logger) formatJSONEntry(e *logEntry) []byte {
	l.writtenCount.Add(1)

	module, _ := e.ctx.Value(ctxModuleKey).(string)
	traceID, _ := e.ctx.Value(ctxTraceIDKey).(string)
	flowID, _ := e.ctx.Value(ctxFlowIDKey).(string)
	attrs, _ := e.ctx.Value(ctxAttrsKey).(map[string]string)

	msg := fmt.Sprintf(e.tmpl, e.args...)
	msg = l.applyMasking(msg, true)

	hookEv := HookEvent{
		Time:     e.t.In(l.loc),
		Level:    e.lvl,
		Module:   module,
		Message:  msg,
		TraceID:  traceID,
		FlowID:   flowID,
		Attrs:    attrs,
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
	}

	entry := jsonEntry{
		Time:    e.t.In(l.loc).Format(time.RFC3339Nano),
		Level:   e.lvl.String(),
		Module:  module,
		Message: msg,
	}

	if l.enableOTEL {
		if tsid := formatTraceSpanID(e.ctx); tsid != "" {
			entry.TraceSpan = tsid
		}
	} else if traceID != "" {
		entry.TraceID = traceID
	}

	if flowID != "" {
		entry.FlowID = flowID
	}
	if len(attrs) > 0 {
		entry.Attrs = attrs
	}

	b, _ := json.Marshal(entry)
	b = append(b, '\n')
	return b
}

// Copyright (c) 2025 Nguyễn Thanh Phương
// This source code is licensed under the MIT License found in the LICENSE file.

// Package unologger - formatters.go
// Cung cấp các bộ định dạng (formatters) mặc định cho log entry.

package unologger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"
)

// TextFormatter định dạng log entry thành chuỗi văn bản.
type TextFormatter struct{}

// Format định dạng HookEvent thành chuỗi văn bản.
func (f *TextFormatter) Format(ev HookEvent) ([]byte, error) {
	ts := ev.Time.Format("2006-01-02 15:04:05.000 MST")
	meta := ""
	if ev.TraceID != "" {
		meta += fmt.Sprintf(" trace=%s", ev.TraceID)
	}
	if ev.FlowID != "" {
		meta += fmt.Sprintf(" flow=%s", ev.FlowID)
	}
	if len(ev.Attrs) > 0 {
		meta += fmt.Sprintf(" attrs=%v", ev.Attrs)
	}
	if len(ev.Fields) > 0 {
		meta += fmt.Sprintf(" fields=%v", ev.Fields)
	}

	line := fmt.Sprintf("%s [%s] (%s)%s %s\n", ts, ev.Level.String(), ev.Module, meta, ev.Message)
	return []byte(line), nil
}

// JSONFormatter định dạng log entry thành chuỗi JSON.
type JSONFormatter struct{}

// Format định dạng HookEvent thành chuỗi JSON.
func (f *JSONFormatter) Format(ev HookEvent) ([]byte, error) {
	type jsonEntry struct {
		Time    string            `json:"time"`
		Level   string            `json:"level"`
		Module  string            `json:"module,omitempty"`
		TraceID string            `json:"trace_id,omitempty"`
		FlowID  string            `json:"flow_id,omitempty"`
		Attrs   map[string]string `json:"attrs,omitempty"`
		Message string            `json:"message"`
		Fields  Fields            `json:"fields,omitempty"`
	}

	entry := jsonEntry{
		Time:    ev.Time.Format(time.RFC3339Nano),
		Level:   ev.Level.String(),
		Module:  ev.Module,
		Message: ev.Message,
		TraceID: ev.TraceID,
		FlowID:  ev.FlowID,
		Attrs:   ev.Attrs,
		Fields:  ev.Fields,
	}

	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(entry); err != nil {
		return nil, err
	}
	// Add newline for easier parsing in log systems
	buf.WriteByte('\n')
	return buf.Bytes(), nil
}

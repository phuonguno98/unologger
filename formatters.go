// Copyright (c) 2025 Nguyễn Thanh Phương
// This source code is licensed under the MIT License found in the LICENSE file.

// Package unologger provides a flexible and feature-rich logging library for Go applications.
// This file defines default formatters for log entries: TextFormatter for human-readable
// plain text output and JSONFormatter for structured JSON output.
// These formatters implement the Formatter interface defined in logger_types.go.

package unologger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"
)

// TextFormatter formats a log entry into a human-readable plain text string.
// It includes timestamp, log level, module, and any associated metadata or fields.
type TextFormatter struct{}

// Format converts a HookEvent into a byte slice representing a plain text log line.
// The output format is: "TIMESTAMP [LEVEL] (MODULE) META MESSAGE\n".
// Metadata (trace, flow, attrs, fields) is appended if present.
func (f *TextFormatter) Format(ev HookEvent) ([]byte, error) {
	// Format the timestamp with milliseconds and timezone.
	ts := ev.Time.Format(time.RFC3339) // Example: 2024-06-15T14:23:45Z07:00.
	// Build metadata string from trace ID, flow ID, attributes, and fields.
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

	// Construct the final log line.
	line := fmt.Sprintf("%s [%s] (%s)%s %s\n", ts, ev.Level.String(), ev.Module, meta, ev.Message)
	return []byte(line), nil
}

// JSONFormatter formats a log entry into a structured JSON string.
// This is ideal for machine-readable logs that can be easily parsed by log management systems.
type JSONFormatter struct{}

// Format converts a HookEvent into a byte slice representing a JSON log line.
// The output includes timestamp, log level, module, message, trace ID, flow ID,
// attributes, and custom fields, all serialized into a JSON object.
func (f *JSONFormatter) Format(ev HookEvent) ([]byte, error) {
	// Define an anonymous struct to control the JSON output structure and field names.
	type jsonEntry struct {
		Time    string            `json:"time"`               // Timestamp in RFC3339Nano format.
		Level   string            `json:"level"`              // Log level string (e.g., "INFO").
		Module  string            `json:"module,omitempty"`   // Module name, omitted if empty.
		TraceID string            `json:"trace_id,omitempty"` // Trace ID, omitted if empty.
		FlowID  string            `json:"flow_id,omitempty"`  // Flow ID, omitted if empty.
		Attrs   map[string]string `json:"attrs,omitempty"`    // Additional attributes, omitted if empty.
		Message string            `json:"message"`            // The main log message.
		Fields  Fields            `json:"fields,omitempty"`   // Custom key-value fields, omitted if empty.
	}

	// Populate the jsonEntry struct from the HookEvent.
	entry := jsonEntry{
		Time:    ev.Time.Format(time.RFC3339), // Example: 2024-06-15T14:23:45Z07:00.
		Level:   ev.Level.String(),
		Module:  ev.Module,
		Message: ev.Message,
		TraceID: ev.TraceID,
		FlowID:  ev.FlowID,
		Attrs:   ev.Attrs,
		Fields:  ev.Fields,
	}

	// Encode the struct to JSON.
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false) // Prevent HTML escaping for cleaner JSON output.
	if err := enc.Encode(entry); err != nil {
		return nil, fmt.Errorf("failed to encode log entry to JSON: %w", err)
	}
	// Add a newline character at the end for easier parsing by log aggregators.
	buf.WriteByte('\n')
	return buf.Bytes(), nil
}

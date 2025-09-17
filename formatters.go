// Copyright (c) 2025 Nguyễn Thanh Phương
// This source code is licensed under the MIT License found in the LICENSE file.

// Package unologger provides a flexible and feature-rich logging library for Go applications.
// This file implements the default formatters for log entries: a human-readable text
// formatter and a machine-readable JSON formatter. Both implement the Formatter interface.

package unologger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"
)

// TextFormatter formats log entries into a human-readable, plain text string.
// This formatter is useful for development environments or console output.
type TextFormatter struct{}

// Format converts a log event into a byte slice representing a single log line.
// The output format is: "TIMESTAMP [LEVEL] (MODULE) KEY=VALUE... MESSAGE\n".
// Metadata like trace ID, flow ID, and other attributes are included as key-value pairs.
func (f *TextFormatter) Format(ev HookEvent) ([]byte, error) {
	// Use a buffer for efficient string building.
	var buf bytes.Buffer

	// Format the timestamp with milliseconds and timezone.
	buf.WriteString(ev.Time.Format(time.RFC3339))
	buf.WriteString(" [")
	buf.WriteString(ev.Level.String())
	buf.WriteString("] (")
	buf.WriteString(ev.Module)
	buf.WriteString(")")

	// Append metadata if present.
	if ev.TraceID != "" {
		buf.WriteString(" trace=")
		buf.WriteString(ev.TraceID)
	}
	if ev.FlowID != "" {
		buf.WriteString(" flow=")
		buf.WriteString(ev.FlowID)
	}
	if len(ev.Attrs) > 0 {
		// A simple, though not perfectly escaped, representation for text logs.
		buf.WriteString(fmt.Sprintf(" attrs=%v", ev.Attrs))
	}
	if len(ev.Fields) > 0 {
		buf.WriteString(fmt.Sprintf(" fields=%v", ev.Fields))
	}

	// Append the main message and a newline.
	buf.WriteString(" ")
	buf.WriteString(ev.Message)
	buf.WriteString("\n")

	return buf.Bytes(), nil
}

// JSONFormatter formats log entries into a structured, machine-readable JSON string.
// This is the recommended formatter for production environments that forward logs
// to a log aggregation service (e.g., ELK, Datadog, Splunk).
type JSONFormatter struct{}

// Format converts a log event into a byte slice representing a JSON object,
// followed by a newline. It includes all metadata from the event.
func (f *JSONFormatter) Format(ev HookEvent) ([]byte, error) {
	// jsonEntry defines the structure of the JSON output.
	// Using `omitempty` ensures that empty fields are not included in the output,
	// keeping the log entries clean.
	type jsonEntry struct {
		Time    string `json:"time"`
		Level   string `json:"level"`
		Module  string `json:"module,omitempty"`
		TraceID string `json:"trace_id,omitempty"`
		FlowID  string `json:"flow_id,omitempty"`
		Attrs   Fields `json:"attrs,omitempty"`
		Message string `json:"message"`
		Fields  Fields `json:"fields,omitempty"`
	}

	// Populate the entry from the event.
	entry := jsonEntry{
		Time:    ev.Time.Format(time.RFC3339),
		Level:   ev.Level.String(),
		Module:  ev.Module,
		Message: ev.Message,
		TraceID: ev.TraceID,
		FlowID:  ev.FlowID,
		Attrs:   ev.Attrs,
		Fields:  ev.Fields,
	}

	// Marshal the entry to JSON.
	// Using a buffer from a sync.Pool could be a future optimization.
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false) // Disable HTML escaping for characters like '<_>_<_>_, '>', '&'_._

	if err := enc.Encode(entry); err != nil {
		return nil, fmt.Errorf("unologger: failed to encode log entry to JSON: %w", err)
	}

	// The encoder already adds a newline, so we don't need to add another.
	return buf.Bytes(), nil
}

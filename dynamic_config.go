// Copyright (c) 2025 Nguyễn Thanh Phương
// This source code is licensed under the MIT License found in the LICENSE file.

// Package unologger provides a flexible and feature-rich logging library for Go applications.
// This file contains methods for dynamically configuring a Logger instance at runtime.
// These thread-safe methods allow for changing the logger's behavior—such as log level,
// output destinations, and masking rules—without requiring an application restart,
// which is crucial for long-running services.

package unologger

import (
	"io"
	"strconv"
	"time"
)

// GetDynamicConfig returns a deep copy of the logger's current dynamic configuration.
// This provides a safe way to inspect the runtime settings without the risk of
// accidental modification to the logger's internal state.
func (l *Logger) GetDynamicConfig() *DynamicConfig {
	l.dynConfig.mu.RLock()
	defer l.dynConfig.mu.RUnlock()
	copyCfg := &DynamicConfig{
		MinLevel:       l.dynConfig.MinLevel,
		RegexRules:     append([]MaskRuleRegex(nil), l.dynConfig.RegexRules...),
		JSONFieldRules: append([]MaskFieldRule(nil), l.dynConfig.JSONFieldRules...),
		Retry:          l.dynConfig.Retry,
		Hooks:          append([]HookFunc(nil), l.dynConfig.Hooks...),
		Batch:          l.dynConfig.Batch,
	}
	return copyCfg
}

// SetMinLevel atomically updates the minimum log level required for a message to be processed.
// Messages with a level lower than this will be discarded.
func (l *Logger) SetMinLevel(level Level) {
	l.dynConfig.mu.Lock()
	defer l.dynConfig.mu.Unlock()
	l.dynConfig.MinLevel = level
	l.minLevel.Store(int32(level))
}

// ShouldLog checks if a message at the given level should be logged based on the
// current minimum log level setting. It is a fast, thread-safe check.
func (l *Logger) ShouldLog(level Level) bool {
	l.dynConfig.mu.RLock()
	defer l.dynConfig.mu.RUnlock()
	return level >= l.dynConfig.MinLevel
}

// SetRegexRules replaces the existing regex-based masking rules with a new set.
// These rules are used to find and mask sensitive information in log messages.
func (l *Logger) SetRegexRules(rules []MaskRuleRegex) {
	l.dynConfig.mu.Lock()
	defer l.dynConfig.mu.Unlock()
	l.dynConfig.RegexRules = rules
	l.regexRules = rules
}

// SetJSONFieldRules replaces the existing JSON field-based masking rules.
// These rules are applied to mask sensitive fields in structured (JSON) log entries
// by matching field keys.
func (l *Logger) SetJSONFieldRules(rules []MaskFieldRule) {
	l.dynConfig.mu.Lock()
	defer l.dynConfig.mu.Unlock()
	l.dynConfig.JSONFieldRules = rules
	l.jsonFieldRules = rules
}

// SetRetryPolicy updates the retry policy for transient output writer errors.
// This policy dictates if and how the logger should attempt to resend failed log batches.
func (l *Logger) SetRetryPolicy(rp RetryPolicy) {
	l.dynConfig.mu.Lock()
	defer l.dynConfig.mu.Unlock()
	l.dynConfig.Retry = rp
	l.retryPolicy = rp
}

// SetHooks replaces the existing list of hook functions with a new set.
// Hooks are functions executed for each log entry, allowing for custom processing.
// If asynchronous hooks are enabled, this method will also ensure the hook runner
// goroutine is active if it's not already.
func (l *Logger) SetHooks(hooks []HookFunc) {
	l.dynConfig.mu.Lock()
	defer l.dynConfig.mu.Unlock()
	l.dynConfig.Hooks = hooks

	l.hooksMu.Lock()
	l.hooks = hooks
	shouldStart := l.hookAsync && l.hookQueueCh == nil && len(hooks) > 0
	l.hooksMu.Unlock()

	if shouldStart {
		l.startHookRunner()
	}
}

// SetBatchConfig updates the batching configuration (size and max wait time).
// This controls how log entries are grouped together before being sent to output writers,
// which can significantly improve performance under high load.
func (l *Logger) SetBatchConfig(bc BatchConfig) {
	l.dynConfig.mu.Lock()
	defer l.dynConfig.mu.Unlock()
	l.dynConfig.Batch = bc
	l.batchSizeA.Store(int64(bc.Size))
	l.batchWaitA.Store(int64(bc.MaxWait))
}

// ResetDynamicConfig reverts the logger's dynamic configuration to a provided initial state.
// This is useful for restoring a known-good configuration at runtime.
func (l *Logger) ResetDynamicConfig(initial *DynamicConfig) {
	l.dynConfig.mu.Lock()
	defer l.dynConfig.mu.Unlock()

	l.dynConfig.MinLevel = initial.MinLevel
	l.dynConfig.RegexRules = append([]MaskRuleRegex(nil), initial.RegexRules...)
	l.dynConfig.JSONFieldRules = append([]MaskFieldRule(nil), initial.JSONFieldRules...)
	l.dynConfig.Retry = initial.Retry
	l.dynConfig.Hooks = append([]HookFunc(nil), initial.Hooks...)
	l.dynConfig.Batch = initial.Batch
	l.minLevel.Store(int32(initial.MinLevel))
	l.regexRules = initial.RegexRules
	l.jsonFieldRules = initial.JSONFieldRules
	l.retryPolicy = initial.Retry

	// Safely update hooks.
	l.hooksMu.Lock()
	l.hooks = initial.Hooks
	l.hooksMu.Unlock()

	l.batchSizeA.Store(int64(initial.Batch.Size))
	l.batchWaitA.Store(int64(initial.Batch.MaxWait))
}

// SetJSONFormat enables or disables JSON-structured logging at runtime.
// When enabled, log entries are formatted as JSON objects.
func (l *Logger) SetJSONFormat(enabled bool) {
	l.jsonFmtFlag.Store(enabled)
	if enabled {
		l.SetFormatter(&JSONFormatter{})
	} else {
		l.SetFormatter(&TextFormatter{})
	}
}

// SetFormatter allows for dynamically changing the log formatter at runtime.
// This can be used to switch between text, JSON, or custom formatters.
func (l *Logger) SetFormatter(f Formatter) {
	l.formatterMu.Lock()
	defer l.formatterMu.Unlock()
	l.formatter = f
}

// SetTimezone updates the timezone used for formatting timestamps in log entries.
// The timezone must be a valid IANA Time Zone database name (e.g., "UTC", "America/New_York").
func (l *Logger) SetTimezone(tz string) error {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return err
	}
	l.locMu.Lock()
	l.loc = loc
	l.locMu.Unlock()
	return nil
}

// SetEnableOTEL enables or disables the automatic extraction of OpenTelemetry
// Trace and Span IDs from the context.
func (l *Logger) SetEnableOTEL(enabled bool) {
	l.enableOTel.Store(enabled)
}

// SetOutputs replaces the logger's output destinations (standard out, standard error,
// and any extra writers). This operation will clear all previously configured extra writers.
func (l *Logger) SetOutputs(stdOut, errOut io.Writer, writers []io.Writer, names []string) {
	l.outputsMu.Lock()
	defer l.outputsMu.Unlock()

	if stdOut != nil {
		l.stdOut = stdOut
	}
	if errOut != nil {
		l.errOut = errOut
	}
	l.extraW = nil
	for i, w := range writers {
		if w == nil {
			continue
		}
		name := ""
		if i < len(names) && names[i] != "" {
			name = names[i]
		} else {
			name = "extra" + strconv.Itoa(i)
		}
		s := writerSink{Name: name, Writer: w}
		if c, ok := w.(io.Closer); ok {
			s.Closer = c
		}
		l.extraW = append(l.extraW, s)
	}
}

// AddExtraWriter adds an additional output writer to the logger.
// If a writer with the same name already exists, it will still be added,
// potentially leading to duplicated output unless the old one is removed first.
// If the name is empty, a default name is assigned.
func (l *Logger) AddExtraWriter(name string, w io.Writer) {
	if w == nil {
		return
	}
	if name == "" {
		name = "extra"
	}
	l.outputsMu.Lock()
	defer l.outputsMu.Unlock()
	s := writerSink{Name: name, Writer: w}
	if c, ok := w.(io.Closer); ok {
		s.Closer = c
	}
	l.extraW = append(l.extraW, s)
}

// RemoveExtraWriter removes an output writer by its name.
// If the writer is found and implements io.Closer, its Close method is called.
// It returns true if a writer was found and removed, and false otherwise.
func (l *Logger) RemoveExtraWriter(name string) bool {
	l.outputsMu.Lock()
	defer l.outputsMu.Unlock()
	idx := -1
	for i, s := range l.extraW {
		if s.Name == name {
			idx = i
			break
		}
	}
	if idx < 0 {
		return false
	}

	// Close the writer if it implements io.Closer.
	if l.extraW[idx].Closer != nil {
		if err := l.extraW[idx].Closer.Close(); err != nil {
			l.writeErrCount.Add(1)
			l.incWriterErr(l.extraW[idx].Name)
		}
	}
	l.extraW = append(l.extraW[:idx], l.extraW[idx+1:]...)
	return true
}

// SetRotation configures log file rotation.
// If a rotation writer was previously configured, it is closed before the new
// configuration is applied. Enabling rotation initializes a new writer based on the
// provided settings.
func (l *Logger) SetRotation(cfg RotationConfig) {
	l.outputsMu.Lock()
	defer l.outputsMu.Unlock()

	// Close the previous rotation writer if it exists.
	if l.rotationSink != nil && l.rotationSink.Closer != nil {
		if err := l.rotationSink.Closer.Close(); err != nil {
			l.writeErrCount.Add(1)
			l.incWriterErr(l.rotationSink.Name)
		}
		l.rotationSink = nil
	}

	if cfg.Enable {
		if w := initRotationWriter(cfg); w != nil {
			l.rotationSink = &writerSink{
				Name:   "rotation",
				Writer: w,
				Closer: w.(io.Closer),
			}
		}
	}
}
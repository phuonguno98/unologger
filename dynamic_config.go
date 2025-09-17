// Copyright (c) 2025 Nguyễn Thanh Phương
// This source code is licensed under the MIT License found in the LICENSE file.

// Package unologger provides a flexible and feature-rich logging library for Go applications.
// This file defines methods to get and update the dynamic configuration of a Logger
// instance at runtime. It allows changing settings like minimum log level, masking rules,
// retry policies, hooks, batching options, output formats, timezones, and log destinations
// without restarting the application.

package unologger

import (
	"io"
	"strconv"
	"time"
)

// GetDynamicConfig returns a copy of the current dynamic configuration of the Logger.
// This allows inspection of the logger's runtime settings without risking modification
// of the internal state.
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

// SetMinLevel updates the minimum log level of the Logger at runtime.
func (l *Logger) SetMinLevel(level Level) {
	l.dynConfig.mu.Lock()
	defer l.dynConfig.mu.Unlock()
	l.dynConfig.MinLevel = level
	l.minLevel.Store(int32(level))
}

// ShouldLog checks if a message at the given level should be logged
// based on the current minimum log level setting.
func (l *Logger) ShouldLog(level Level) bool {
	l.dynConfig.mu.RLock()
	defer l.dynConfig.mu.RUnlock()
	return level >= l.dynConfig.MinLevel
}

// SetRegexRules updates the regex-based masking rules at runtime.
// These rules are used to mask sensitive information in log messages.
func (l *Logger) SetRegexRules(rules []MaskRuleRegex) {
	l.dynConfig.mu.Lock()
	defer l.dynConfig.mu.Unlock()
	l.dynConfig.RegexRules = rules
	l.regexRules = rules
}

// SetJSONFieldRules updates the JSON field masking rules at runtime.
// These rules are applied to mask sensitive fields in structured log entries.
func (l *Logger) SetJSONFieldRules(rules []MaskFieldRule) {
	l.dynConfig.mu.Lock()
	defer l.dynConfig.mu.Unlock()
	l.dynConfig.JSONFieldRules = rules
	l.jsonFieldRules = rules
}

// SetRetryPolicy updates the retry policy for transient logging failures at runtime.
// This controls how the logger attempts to resend failed log entries.
func (l *Logger) SetRetryPolicy(rp RetryPolicy) {
	l.dynConfig.mu.Lock()
	defer l.dynConfig.mu.Unlock()
	l.dynConfig.Retry = rp
	l.retryPolicy = rp
}

// SetHooks updates the list of hook functions that are called on each log entry.
// If hookAsync is enabled, it ensures the hook processing goroutine is started if needed.
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

// SetBatchConfig updates the batching configuration for log entries at runtime.
// This controls how log entries are grouped and sent in batches to improve performance.
func (l *Logger) SetBatchConfig(bc BatchConfig) {
	l.dynConfig.mu.Lock()
	defer l.dynConfig.mu.Unlock()
	l.dynConfig.Batch = bc
	l.batchSize = bc.Size
	l.batchWait = bc.MaxWait
	l.batchSizeA.Store(int64(bc.Size))
	l.batchWaitA.Store(int64(bc.MaxWait))
}

// ResetDynamicConfig resets the Logger's dynamic configuration to the provided initial settings.
// This allows restoring a known configuration state at runtime.
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

	// Update hooks safely.
	l.hooksMu.Lock()
	l.hooks = initial.Hooks
	l.hooksMu.Unlock()

	l.batchSize = initial.Batch.Size
	l.batchWait = initial.Batch.MaxWait
	l.batchSizeA.Store(int64(initial.Batch.Size))
	l.batchWaitA.Store(int64(initial.Batch.MaxWait))
}

// SetJSONFormat enables or disables JSON log format at runtime.
func (l *Logger) SetJSONFormat(enabled bool) {
	l.jsonFmtFlag.Store(enabled)
}

// SetTimezone updates the timezone used for timestamps in log entries.
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

// SetEnableOTEL enables or disables OpenTelemetry trace ID extraction for log entries.
func (l *Logger) SetEnableOTEL(enabled bool) {
	l.enableOTel.Store(enabled)
}

// SetOutputs updates the output destinations for the Logger at runtime.
// It allows changing the standard output, error output, and additional writers dynamically.
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

// AddExtraWriter adds an additional output writer to the Logger at runtime.
// If a writer with the same name already exists, it will not be added again.
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

// RemoveExtraWriter removes an additional output writer by name from the Logger at runtime.
// If the writer is found and removed, it returns true; otherwise, it returns false.
// If the writer implements io.Closer, it will be closed when removed.
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

	// Close writer if it implements io.Closer.
	if l.extraW[idx].Closer != nil {
		if err := l.extraW[idx].Closer.Close(); err != nil {
			l.writeErrCount.Add(1)
			l.incWriterErr(l.extraW[idx].Name)
		}
	}
	l.extraW = append(l.extraW[:idx], l.extraW[idx+1:]...)
	return true
}

// SetRotation configures log file rotation settings at runtime.
// If rotation is enabled, it initializes the rotation writer accordingly.
// If there was a previous rotation writer, it will be closed before applying the new settings.
func (l *Logger) SetRotation(cfg RotationConfig) {
	l.outputsMu.Lock()
	defer l.outputsMu.Unlock()

	// Close previous rotation writer if exists.
	if l.rotationSink != nil && l.rotationSink.Closer != nil {
		if err := l.rotationSink.Closer.Close(); err != nil {
			l.writeErrCount.Add(1)
			l.incWriterErr(l.rotationSink.Name)
		}
		l.rotationSink = nil
	}
	l.rotation = cfg
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

// Copyright (c) 2025 Nguyễn Thanh Phương
// This source code is licensed under the MIT License found in the LICENSE file.

// Package unologger - dynamic_config.go
// Cung cấp cơ chế cấu hình động cho Logger, cho phép thay đổi tham số ghi log khi hệ thống đang chạy.
// Điều này hữu ích khi muốn điều chỉnh mức log (bao gồm cả FATAL), bật/tắt masking dữ liệu nhạy cảm,
// hoặc thay đổi writer mà không cần khởi động lại dịch vụ.

package unologger

import (
	"io"
	"strconv"
	"time"
)

// GetDynamicConfig trả về bản sao cấu hình động hiện tại.
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

// SetMinLevel cập nhật min-level khi runtime, áp dụng tức thời.
func (l *Logger) SetMinLevel(level Level) {
	l.dynConfig.mu.Lock()
	defer l.dynConfig.mu.Unlock()
	l.dynConfig.MinLevel = level
	l.minLevel.Store(int32(level))
}

// ShouldLog trả về true nếu level được phép theo thiết lập hiện tại.
func (l *Logger) ShouldLog(level Level) bool {
	l.dynConfig.mu.RLock()
	defer l.dynConfig.mu.RUnlock()
	return level >= l.dynConfig.MinLevel
}

// SetRegexRules cập nhật quy tắc masking regex khi runtime.
func (l *Logger) SetRegexRules(rules []MaskRuleRegex) {
	l.dynConfig.mu.Lock()
	defer l.dynConfig.mu.Unlock()
	l.dynConfig.RegexRules = rules
	l.regexRules = rules
}

// SetJSONFieldRules cập nhật quy tắc masking theo tên field JSON khi runtime.
func (l *Logger) SetJSONFieldRules(rules []MaskFieldRule) {
	l.dynConfig.mu.Lock()
	defer l.dynConfig.mu.Unlock()
	l.dynConfig.JSONFieldRules = rules
	l.jsonFieldRules = rules
}

// SetRetryPolicy cập nhật retry policy khi runtime.
func (l *Logger) SetRetryPolicy(rp RetryPolicy) {
	l.dynConfig.mu.Lock()
	defer l.dynConfig.mu.Unlock()
	l.dynConfig.Retry = rp
	l.retryPolicy = rp
}

// SetHooks cập nhật danh sách hooks; sẽ tự khởi động hook runner nếu cần.
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

// SetBatchConfig cập nhật batch size/max-wait; dùng atomic để worker thấy ngay.
func (l *Logger) SetBatchConfig(bc BatchConfig) {
	l.dynConfig.mu.Lock()
	defer l.dynConfig.mu.Unlock()
	l.dynConfig.Batch = bc
	l.batchSize = bc.Size
	l.batchWait = bc.MaxWait
	l.batchSizeA.Store(int64(bc.Size))
	l.batchWaitA.Store(int64(bc.MaxWait))
}

// ResetDynamicConfig khôi phục cấu hình động về trạng thái ban đầu khi init.
func (l *Logger) ResetDynamicConfig(initial *DynamicConfig) {
	l.dynConfig.mu.Lock()
	defer l.dynConfig.mu.Unlock()

	l.dynConfig.MinLevel = initial.MinLevel
	l.dynConfig.RegexRules = append([]MaskRuleRegex(nil), initial.RegexRules...)
	l.dynConfig.JSONFieldRules = append([]MaskFieldRule(nil), initial.JSONFieldRules...)
	l.dynConfig.Retry = initial.Retry
	l.dynConfig.Hooks = append([]HookFunc(nil), initial.Hooks...)
	l.dynConfig.Batch = initial.Batch

	// Áp dụng lại vào logger
	l.minLevel.Store(int32(initial.MinLevel))
	l.regexRules = initial.RegexRules
	l.jsonFieldRules = initial.JSONFieldRules
	l.retryPolicy = initial.Retry

	// Cập nhật hooks an toàn
	l.hooksMu.Lock()
	l.hooks = initial.Hooks
	l.hooksMu.Unlock()

	l.batchSize = initial.Batch.Size
	l.batchWait = initial.Batch.MaxWait
	l.batchSizeA.Store(int64(initial.Batch.Size))
	l.batchWaitA.Store(int64(initial.Batch.MaxWait))
}

// SetJSONFormat bật/tắt định dạng JSON khi runtime.
func (l *Logger) SetJSONFormat(enabled bool) {
	l.jsonFmtFlag.Store(enabled)
}

// SetTimezone đổi múi giờ khi runtime.
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

// SetEnableOTEL bật/tắt tích hợp OpenTelemetry khi runtime.
func (l *Logger) SetEnableOTEL(enabled bool) {
	l.enableOTEL.Store(enabled)
}

// SetOutputs thay đổi stdout/stderr và danh sách writer phụ khi runtime.
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

// AddExtraWriter thêm writer phụ mới theo tên.
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

// RemoveExtraWriter xóa writer phụ theo tên; trả về true nếu xóa thành công.
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
	// cố gắng đóng nếu có
	if l.extraW[idx].Closer != nil {
		if err := l.extraW[idx].Closer.Close(); err != nil {
			l.writeErrCount.Add(1)
			l.incWriterErr(l.extraW[idx].Name)
		}
	}
	l.extraW = append(l.extraW[:idx], l.extraW[idx+1:]...)
	return true
}

// SetRotation cập nhật cấu hình rotation và writer xoay file khi runtime.
func (l *Logger) SetRotation(cfg RotationConfig) {
	l.outputsMu.Lock()
	defer l.outputsMu.Unlock()

	// Đóng writer cũ nếu có
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

// Copyright (c) 2025 Nguyễn Thanh Phương
// This source code is licensed under the MIT License found in the LICENSE file.

// Package unologger - dynamic_config.go
// Cung cấp cơ chế cấu hình động cho Logger, cho phép thay đổi tham số ghi log khi hệ thống đang chạy.
// Điều này hữu ích khi muốn điều chỉnh mức log (bao gồm cả FATAL), bật/tắt masking dữ liệu nhạy cảm,
// hoặc thay đổi writer mà không cần khởi động lại dịch vụ.

package unologger

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

// SetMinLevel cập nhật cấp độ log tối thiểu khi runtime.
// Hỗ trợ đầy đủ các cấp độ: DEBUG, INFO, WARN, ERROR, FATAL.
func (l *Logger) SetMinLevel(level Level) {
	l.dynConfig.mu.Lock()
	defer l.dynConfig.mu.Unlock()
	l.dynConfig.MinLevel = level
	l.minLevel.Store(int32(level))
}

// ShouldLog kiểm tra xem một log ở mức level cho trước có được ghi hay không
// dựa trên cấu hình động hiện tại.
func (l *Logger) ShouldLog(level Level) bool {
	l.dynConfig.mu.RLock()
	defer l.dynConfig.mu.RUnlock()
	return level >= l.dynConfig.MinLevel
}

// SetRegexRules cập nhật danh sách regex masking khi runtime.
func (l *Logger) SetRegexRules(rules []MaskRuleRegex) {
	l.dynConfig.mu.Lock()
	defer l.dynConfig.mu.Unlock()
	l.dynConfig.RegexRules = rules
	l.regexRules = rules
}

// SetJSONFieldRules cập nhật danh sách field-level masking khi runtime.
func (l *Logger) SetJSONFieldRules(rules []MaskFieldRule) {
	l.dynConfig.mu.Lock()
	defer l.dynConfig.mu.Unlock()
	l.dynConfig.JSONFieldRules = rules
	l.jsonFieldRules = rules
}

// SetRetryPolicy cập nhật retryPolicy khi runtime.
func (l *Logger) SetRetryPolicy(rp RetryPolicy) {
	l.dynConfig.mu.Lock()
	defer l.dynConfig.mu.Unlock()
	l.dynConfig.Retry = rp
	l.retryPolicy = rp
}

// SetHooks cập nhật danh sách hooks khi runtime.
func (l *Logger) SetHooks(hooks []HookFunc) {
	l.dynConfig.mu.Lock()
	defer l.dynConfig.mu.Unlock()
	l.dynConfig.Hooks = hooks
	l.hooks = hooks
}

// SetBatchConfig cập nhật batch size/time khi runtime.
func (l *Logger) SetBatchConfig(bc BatchConfig) {
	l.dynConfig.mu.Lock()
	defer l.dynConfig.mu.Unlock()
	l.dynConfig.Batch = bc
	l.batchSize = bc.Size
	l.batchWait = bc.MaxWait
}

// ResetDynamicConfig khôi phục cấu hình động về giá trị ban đầu khi init.
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
	l.hooks = initial.Hooks
	l.batchSize = initial.Batch.Size
	l.batchWait = initial.Batch.MaxWait
}

// Copyright (c) 2025 Nguyễn Thanh Phương
// This source code is licensed under the MIT License found in the LICENSE file.

// Package unologger - masking.go
// Cung cấp cơ chế che (mask) dữ liệu nhạy cảm trong log.
// Hỗ trợ:
//   - Mask theo regex (áp dụng cho log text hoặc JSON).
//   - Mask theo tên trường (field-level) trong JSON.
// Mục tiêu: tránh ghi trực tiếp thông tin nhạy cảm (số thẻ, email, token, mật khẩu) vào log.

package unologger

import (
	"bytes"
	"encoding/json"
	"regexp"
)

// applyMasking áp dụng tất cả quy tắc masking cho message text hoặc JSON.
// Nếu jsonMode=true, sẽ thử parse JSON và mask theo field-level trước, sau đó fallback regex.
// Nếu jsonMode=false, chỉ áp dụng regex masking.
// Lấy rules từ dynamic config hiện tại của Logger.
func (l *Logger) applyMasking(msg string, jsonMode bool) string {
	l.dynConfig.mu.RLock()
	regexRules := l.dynConfig.RegexRules
	jsonFieldRules := l.dynConfig.JSONFieldRules
	l.dynConfig.mu.RUnlock()

	if jsonMode {
		if masked, ok := maskJSONFieldsWithRules(msg, jsonFieldRules); ok {
			// Sau khi mask field-level, vẫn áp dụng regex để che mờ phần còn lại
			return maskRegexWithRules(masked, regexRules)
		}
		// Nếu parse JSON thất bại, fallback regex
	}
	return maskRegexWithRules(msg, regexRules)
}

// maskRegexWithRules áp dụng tất cả regexRules lên chuỗi.
func maskRegexWithRules(s string, rules []MaskRuleRegex) string {
	if len(rules) == 0 {
		return s
	}
	masked := s
	for _, rule := range rules {
		if rule.Pattern != nil {
			masked = rule.Pattern.ReplaceAllString(masked, rule.Replacement)
		}
	}
	return masked
}

// maskJSONFieldsWithRules parse chuỗi JSON và che mờ các field có tên khớp JSONFieldRules.
// Trả về (chuỗi JSON đã mask, true) nếu parse thành công, ngược lại ("", false).
func maskJSONFieldsWithRules(s string, rules []MaskFieldRule) (string, bool) {
	if len(rules) == 0 {
		return s, true
	}

	// Parse JSON thành interface{}
	var data interface{}
	dec := json.NewDecoder(bytes.NewBufferString(s))
	dec.UseNumber()
	if err := dec.Decode(&data); err != nil {
		return "", false
	}

	// Áp dụng mask đệ quy
	maskJSONValueWithRules(&data, rules)

	// Encode lại thành JSON
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(data); err != nil {
		return "", false
	}
	// Xóa newline cuối do Encoder thêm
	out := bytes.TrimRight(buf.Bytes(), "\n")
	return string(out), true
}

// maskJSONValueWithRules áp dụng mask cho giá trị JSON (đệ quy).
func maskJSONValueWithRules(v *interface{}, rules []MaskFieldRule) {
	switch val := (*v).(type) {
	case map[string]interface{}:
		for k, sub := range val {
			if shouldMaskKeyWithRules(k, rules) {
				val[k] = getMaskReplacementForKeyWithRules(k, rules)
			} else {
				maskJSONValueWithRules(&sub, rules)
				val[k] = sub
			}
		}
	case []interface{}:
		for i, sub := range val {
			maskJSONValueWithRules(&sub, rules)
			val[i] = sub
		}
	}
}

// shouldMaskKeyWithRules kiểm tra xem key có nằm trong danh sách cần mask không.
func shouldMaskKeyWithRules(key string, rules []MaskFieldRule) bool {
	for _, rule := range rules {
		for _, rk := range rule.Keys {
			if rk == key {
				return true
			}
		}
	}
	return false
}

// getMaskReplacementForKeyWithRules trả về replacement cho key cần mask.
func getMaskReplacementForKeyWithRules(key string, rules []MaskFieldRule) string {
	for _, rule := range rules {
		for _, rk := range rule.Keys {
			if rk == key {
				return rule.Replacement
			}
		}
	}
	return "***"
}

// compileMaskRegexes tiện ích để compile pattern string thành regexp.
// Trả về slice MaskRuleRegex đã compile thành công.
func compileMaskRegexes(patterns map[string]string) []MaskRuleRegex {
	var rules []MaskRuleRegex
	for pat, repl := range patterns {
		if re, err := regexp.Compile(pat); err == nil {
			rules = append(rules, MaskRuleRegex{Pattern: re, Replacement: repl})
		}
	}
	return rules
}

// Copyright (c) 2025 Nguyễn Thanh Phương
// This source code is licensed under the MIT License found in the LICENSE file.

// Package unologger provides a flexible and feature-rich logging library for Go applications.
// This file implements the logic for masking sensitive data within log messages.
// It supports both regex-based pattern matching and structured JSON field masking
// to prevent credentials, personal information, and other secrets from being logged.

package unologger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
)

// applyMasking applies all configured masking rules to a log message string.
//
// The masking process follows a specific order:
//  1. If in JSON mode, it first attempts to mask specific fields within the JSON structure.
//  2. It then applies regex-based masking to the result (either the original string
//     or the JSON-masked string).
//
// This ensures that regex rules can still apply even after field-level masking.
func (l *Logger) applyMasking(msg string, jsonMode bool) string {
	l.dynConfig.mu.RLock()
	regexRules := l.dynConfig.RegexRules
	jsonFieldRules := l.dynConfig.JSONFieldRules
	l.dynConfig.mu.RUnlock()

	if jsonMode {
		// Attempt to mask JSON fields first.
		if maskedJSON, ok := maskJSONFieldsWithRules(msg, jsonFieldRules); ok {
			// If successful, apply regex rules to the already-masked JSON string.
			return maskRegexWithRules(maskedJSON, regexRules)
		}
		// If JSON parsing failed, fall through to apply regex masking to the original string.
	}

	// For non-JSON logs, or as a fallback for failed JSON parsing.
	return maskRegexWithRules(msg, regexRules)
}

// maskRegexWithRules is a helper that applies a slice of regex rules to a string.
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

// maskJSONFieldsWithRules parses a JSON string and masks the values of any fields
// that match the configured rules. It returns the modified JSON string.
// If the input string is not valid JSON, it returns the original string and false.
func maskJSONFieldsWithRules(s string, rules []MaskFieldRule) (string, bool) {
	if len(rules) == 0 {
		return s, true
	}

	var data interface{}
	// Use a decoder with UseNumber() to prevent large integer IDs from being
	// converted to float64, which could cause a loss of precision.
	dec := json.NewDecoder(bytes.NewBufferString(s))
	dec.UseNumber()
	if err := dec.Decode(&data); err != nil {
		return s, false // Not a valid JSON string.
	}

	// Recursively traverse the data structure and mask values.
	maskJSONValueWithRules(&data, rules)

	// Re-encode the data structure back to a JSON string.
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false) // Prevent characters like '<' and '>' from being escaped.
	if err := enc.Encode(data); err != nil {
		// This should be a rare event, but if it happens, return the original string.
		return s, false
	}
	// Trim the trailing newline added by the encoder.
	out := bytes.TrimRight(buf.Bytes(), "\n")
	return string(out), true
}

// maskJSONValueWithRules recursively traverses a data structure (map or slice)
// and applies masking rules. It takes a pointer to an interface{} to allow
// in-place modification of the underlying data.
func maskJSONValueWithRules(v *interface{}, rules []MaskFieldRule) {
	switch val := (*v).(type) {
	case map[string]interface{}:
		for k, subVal := range val {
			if shouldMaskKeyWithRules(k, rules) {
				val[k] = getMaskReplacementForKeyWithRules(k, rules)
			} else {
				// The value might be another map or slice, so recurse.
				maskJSONValueWithRules(&subVal, rules)
				val[k] = subVal
			}
		}
	case []interface{}:
		for i, subVal := range val {
			// Recurse into each element of the slice.
			maskJSONValueWithRules(&subVal, rules)
			val[i] = subVal
		}
	}
}

// shouldMaskKeyWithRules checks if a given key matches any of the configured masking rules.
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

// getMaskReplacementForKeyWithRules finds the corresponding replacement string for a key.
func getMaskReplacementForKeyWithRules(key string, rules []MaskFieldRule) string {
	for _, rule := range rules {
		for _, rk := range rule.Keys {
			if rk == key {
				return rule.Replacement
			}
		}
	}
	return "***" // Fallback replacement.
}

// compileMaskRegexes is an internal helper that compiles a map of string patterns
// into a slice of MaskRuleRegex. This is typically called once during logger initialization.
// If a pattern is an invalid regex, an error is printed to os.Stderr and the pattern is skipped.
func compileMaskRegexes(patterns map[string]string) []MaskRuleRegex {
	if len(patterns) == 0 {
		return nil
	}
	rules := make([]MaskRuleRegex, 0, len(patterns))
	for pat, repl := range patterns {
		if re, err := regexp.Compile(pat); err == nil {
			rules = append(rules, MaskRuleRegex{Pattern: re, Replacement: repl})
		} else {
			// Log initialization error directly to stderr.
			fmt.Fprintf(os.Stderr, "unologger: failed to compile regex masking pattern '%s': %v\n", pat, err)
		}
	}
	return rules
}

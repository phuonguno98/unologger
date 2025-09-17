// Copyright (c) 2025 Nguyễn Thanh Phương
// This source code is licensed under the MIT License found in the LICENSE file.

// Package unologger provides a flexible and feature-rich logging library for Go applications.
// This file implements the sensitive data masking mechanisms for log entries.
// It supports two primary masking strategies:
//   - Regex-based masking: Applies regular expressions to obscure patterns in text or JSON logs.
//   - Field-level JSON masking: Masks specific fields within JSON log structures.
// The goal is to prevent sensitive information (e.g., credit card numbers, emails, tokens, passwords)
// from being written directly into logs, enhancing security and compliance.
package unologger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"os"
)

// applyMasking applies configured masking rules to a log message.
// It first attempts field-level JSON masking if jsonMode is true and then
// applies regex-based masking to the result. If JSON parsing fails, it falls back
// to only regex masking.
func (l *Logger) applyMasking(msg string, jsonMode bool) string {
	l.dynConfig.mu.RLock() // Acquire read lock for dynamic configuration.
	regexRules := l.dynConfig.RegexRules
	jsonFieldRules := l.dynConfig.JSONFieldRules
	l.dynConfig.mu.RUnlock() // Release read lock.

	if jsonMode {
		// Attempt to mask JSON fields first.
		if maskedJSON, ok := maskJSONFieldsWithRules(msg, jsonFieldRules); ok {
			// If JSON parsing and field masking were successful, apply regex rules to the result.
			return maskRegexWithRules(maskedJSON, regexRules)
		}
		// If JSON parsing failed, fall through to regex masking only.
	}
	// Apply regex rules to the message (either original or after failed JSON parsing).
	return maskRegexWithRules(msg, regexRules)
}

// maskRegexWithRules applies all provided regex rules to a given string.
// It iterates through each rule and replaces matched patterns with their specified replacement string.
func maskRegexWithRules(s string, rules []MaskRuleRegex) string {
	if len(rules) == 0 {
		return s // No regex rules to apply.
	}
	masked := s
	for _, rule := range rules {
		if rule.Pattern != nil {
			// Replace all occurrences of the pattern in the string.
			masked = rule.Pattern.ReplaceAllString(masked, rule.Replacement)
		}
	}
	return masked
}

// maskJSONFieldsWithRules parses a JSON string and masks values of fields
// that match the provided JSONFieldRules.
// It returns the masked JSON string and true if parsing and masking were successful.
// If JSON parsing fails, it returns an empty string and false.
func maskJSONFieldsWithRules(s string, rules []MaskFieldRule) (string, bool) {
	if len(rules) == 0 {
		return s, true // No JSON field rules to apply.
	}

	// Parse the JSON string into a generic interface{}.
	var data interface{}
	dec := json.NewDecoder(bytes.NewBufferString(s))
	dec.UseNumber() // Preserve numbers as json.Number instead of float64.
	if err := dec.Decode(&data); err != nil {
		return "", false // Failed to parse JSON.
	}

	// Recursively apply masking to the parsed JSON data.
	maskJSONValueWithRules(&data, rules)

	// Encode the modified data back into a JSON string.
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false) // Prevent HTML escaping for cleaner JSON output.
	if err := enc.Encode(data); err != nil {
		return "", false // Failed to encode JSON.
	}
	// Trim the trailing newline added by json.Encoder for single-line log entries.
	out := bytes.TrimRight(buf.Bytes(), "\n")
	return string(out), true
}

// maskJSONValueWithRules recursively applies masking to JSON values.
// It traverses maps (objects) and slices (arrays) to find fields that need masking.
func maskJSONValueWithRules(v *interface{}, rules []MaskFieldRule) {
	switch val := (*v).(type) {
	case map[string]interface{}:
		// Iterate over map (JSON object) fields.
		for k, sub := range val {
			if shouldMaskKeyWithRules(k, rules) {
				// If the key should be masked, replace its value.
				val[k] = getMaskReplacementForKeyWithRules(k, rules)
			} else {
				// Recursively mask nested values.
				maskJSONValueWithRules(&sub, rules)
				val[k] = sub // Update the map with the (potentially masked) sub-value.
			}
		}
	case []interface{}:
		// Iterate over slice (JSON array) elements.
		for i, sub := range val {
			// Recursively mask each element.
			maskJSONValueWithRules(&sub, rules)
			val[i] = sub // Update the slice with the (potentially masked) sub-value.
		}
	}
}

// shouldMaskKeyWithRules checks if a given key should be masked based on the provided rules.
// It returns true if the key is found in any of the MaskFieldRule's Keys list.
func shouldMaskKeyWithRules(key string, rules []MaskFieldRule) bool {
	for _, rule := range rules {
		for _, rk := range rule.Keys {
			if rk == key {
				return true // Key found in masking rules.
			}
		}
	}
	return false // Key does not need masking.
}

// getMaskReplacementForKeyWithRules retrieves the replacement string for a given masked key.
// It returns the replacement string from the first matching rule, or a default "***" if no rule matches.
func getMaskReplacementForKeyWithRules(key string, rules []MaskFieldRule) string {
	for _, rule := range rules {
		for _, rk := range rule.Keys {
			if rk == key {
				return rule.Replacement // Return the specific replacement for this rule.
			}
		}
	}
	return "***" // Default replacement if no specific rule is found.
}

// compileMaskRegexes compiles a map of regex pattern strings to replacement strings
// into a slice of MaskRuleRegex. Invalid regex patterns are ignored.
func compileMaskRegexes(patterns map[string]string) []MaskRuleRegex {
	var rules []MaskRuleRegex
	for pat, repl := range patterns {
		if re, err := regexp.Compile(pat); err == nil {
			// Successfully compiled regex, add to rules.
			rules = append(rules, MaskRuleRegex{Pattern: re, Replacement: repl})
		} else {
			// Log or handle the error for invalid regex pattern if necessary.
			// For now, invalid patterns are simply skipped.
			fmt.Fprintf(os.Stderr, "unologger: failed to compile regex pattern '%s': %v\n", pat, err)
		}
	}
	return rules
}
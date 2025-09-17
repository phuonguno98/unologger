// Copyright (c) 2025 Nguyễn Thanh Phương
// This source code is licensed under the MIT License found in the LICENSE file.

// Package unologger provides a flexible and feature-rich logging library for Go applications.
// This file implements the log file rotation mechanism, which helps manage log file sizes
// and retention. It utilizes the `lumberjack` library to rotate log files based on size,
// age, and the number of backup files, ensuring efficient log storage and management.

package unologger

import (
	"io"

	"gopkg.in/natefinch/lumberjack.v2"
)

// initRotationWriter creates and returns an io.Writer that handles log file rotation.
// It uses the `lumberjack.Logger` to manage file rotation based on the provided RotationConfig.
// Returns nil if log rotation is not enabled or if the filename is not specified in the config.
func initRotationWriter(cfg RotationConfig) io.Writer {
	// Check if rotation is enabled and a filename is provided.
	if !cfg.Enable || cfg.Filename == "" {
		return nil // Rotation is not active without these settings.
	}
	// Initialize and return a new lumberjack.Logger instance.
	return &lumberjack.Logger{
		Filename:   cfg.Filename,   // The base filename for the log file.
		MaxSize:    cfg.MaxSizeMB,  // Maximum size in megabytes before the log file is rotated.
		MaxAge:     cfg.MaxAge,     // Maximum number of days to retain old log files.
		MaxBackups: cfg.MaxBackups, // Maximum number of old log files to keep.
		Compress:   cfg.Compress,   // Whether to compress rotated log files.
	}
}

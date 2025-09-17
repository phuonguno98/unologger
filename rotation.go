// Copyright (c) 2025 Nguyễn Thanh Phương
// This source code is licensed under the MIT License found in the LICENSE file.

// Package unologger provides a flexible and feature-rich logging library for Go applications.
// This file implements the log rotation functionality by integrating with the
// gopkg.in/natefinch/lumberjack.v2 library. It provides a helper function to
// create a rotating log writer from the logger's configuration.

package unologger

import (
	"io"

	"gopkg.in/natefinch/lumberjack.v2"
)

// initRotationWriter creates and returns an io.Writer for log file rotation
// based on the provided configuration.
// It returns nil if rotation is disabled or if no filename is specified.
// This function serves as an internal factory for the lumberjack.Logger.
func initRotationWriter(cfg RotationConfig) io.Writer {
	if !cfg.Enable || cfg.Filename == "" {
		return nil
	}
	// The lumberjack.Logger is an io.WriteCloser that handles all rotation logic.
	return &lumberjack.Logger{
		Filename:   cfg.Filename,
		MaxSize:    cfg.MaxSizeMB,
		MaxAge:     cfg.MaxAge,
		MaxBackups: cfg.MaxBackups,
		Compress:   cfg.Compress,
	}
}

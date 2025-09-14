// Copyright (c) 2025 Nguyễn Thanh Phương
// This source code is licensed under the MIT License found in the LICENSE file.

// Package unologger - init.go
// Khởi tạo logger mặc định (globalLogger) và cung cấp hàm tạo logger riêng (detached logger).

package unologger

import (
	"io"
	"os"
	"time"
)

// InitLogger khởi tạo globalLogger với cấu hình cơ bản.
//   - minLevel: cấp độ log tối thiểu (DEBUG, INFO, WARN, ERROR, FATAL).
//   - timezone: tên múi giờ (ví dụ: "Asia/Ho_Chi_Minh").
//
// Nếu timezone không hợp lệ, sẽ dùng UTC.
// Logger mặc định ghi ra stdout (log thường) và stderr (log lỗi).
func InitLogger(minLevel Level, timezone string) {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		loc = time.UTC
	}
	globalLogger = &Logger{
		stdOut:      os.Stdout,
		errOut:      os.Stderr,
		loc:         loc,
		jsonFmt:     false,
		ch:          make(chan *logEntry, 1024),
		workers:     1,
		nonBlocking: false,
		dropOldest:  false,
		batchSize:   1,
		batchWait:   time.Second,
		retryPolicy: RetryPolicy{},
		minLevel:    atomicLevel{},
	}
	globalLogger.minLevel.Store(int32(minLevel))
	globalLogger.dynConfig.MinLevel = minLevel
	globalLogger.startWorkers()
}

// InitLoggerWithConfig khởi tạo globalLogger với Config tùy chỉnh.
func InitLoggerWithConfig(cfg Config) {
	// Đảm bảo writer chính luôn hợp lệ
	if cfg.Stdout == nil {
		cfg.Stdout = os.Stdout
	}
	if cfg.Stderr == nil {
		cfg.Stderr = os.Stderr
	}

	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		loc = time.UTC
	}

	// Nếu RegexPatternMap có dữ liệu, compile thành RegexRules
	if len(cfg.RegexPatternMap) > 0 {
		cfg.RegexRules = append(cfg.RegexRules, compileMaskRegexes(cfg.RegexPatternMap)...)
	}

	l := &Logger{
		stdOut:         cfg.Stdout,
		errOut:         cfg.Stderr,
		loc:            loc,
		jsonFmt:        cfg.JSON,
		ch:             make(chan *logEntry, cfg.Buffer),
		workers:        cfg.Workers,
		nonBlocking:    cfg.NonBlocking,
		dropOldest:     cfg.DropOldest,
		batchSize:      cfg.Batch.Size,
		batchWait:      cfg.Batch.MaxWait,
		retryPolicy:    cfg.Retry,
		hooks:          cfg.Hooks,
		hookAsync:      cfg.Hook.Async,
		hookWorkers:    cfg.Hook.Workers,
		hookQueue:      cfg.Hook.Queue,
		hookTimeout:    cfg.Hook.Timeout,
		regexRules:     cfg.RegexRules,
		jsonFieldRules: cfg.JSONFieldRules,
		enableOTEL:     cfg.EnableOTEL,
		rotation:       cfg.Rotation,
	}
	l.minLevel.Store(int32(cfg.MinLevel))
	l.dynConfig.MinLevel = cfg.MinLevel
	l.dynConfig.RegexRules = cfg.RegexRules
	l.dynConfig.JSONFieldRules = cfg.JSONFieldRules
	l.dynConfig.Retry = cfg.Retry
	l.dynConfig.Hooks = cfg.Hooks
	l.dynConfig.Batch = cfg.Batch

	// Khởi tạo rotation writer nếu bật
	if cfg.Rotation.Enable {
		if w := initRotationWriter(cfg.Rotation); w != nil {
			l.rotationSink = &writerSink{
				Name:   "rotation",
				Writer: w,
				Closer: w.(io.Closer),
			}
		}
	}

	globalLogger = l
	globalLogger.startWorkers()
	if globalLogger.hookAsync {
		globalLogger.startHookRunner()
	}
}

// NewDetachedLogger tạo một logger riêng (không ảnh hưởng globalLogger) với Config tùy chỉnh.
func NewDetachedLogger(cfg Config) *Logger {
	// Đảm bảo writer chính luôn hợp lệ
	if cfg.Stdout == nil {
		cfg.Stdout = os.Stdout
	}
	if cfg.Stderr == nil {
		cfg.Stderr = os.Stderr
	}

	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		loc = time.UTC
	}

	// Nếu RegexPatternMap có dữ liệu, compile thành RegexRules
	if len(cfg.RegexPatternMap) > 0 {
		cfg.RegexRules = append(cfg.RegexRules, compileMaskRegexes(cfg.RegexPatternMap)...)
	}

	l := &Logger{
		stdOut:         cfg.Stdout,
		errOut:         cfg.Stderr,
		loc:            loc,
		jsonFmt:        cfg.JSON,
		ch:             make(chan *logEntry, cfg.Buffer),
		workers:        cfg.Workers,
		nonBlocking:    cfg.NonBlocking,
		dropOldest:     cfg.DropOldest,
		batchSize:      cfg.Batch.Size,
		batchWait:      cfg.Batch.MaxWait,
		retryPolicy:    cfg.Retry,
		hooks:          cfg.Hooks,
		hookAsync:      cfg.Hook.Async,
		hookWorkers:    cfg.Hook.Workers,
		hookQueue:      cfg.Hook.Queue,
		hookTimeout:    cfg.Hook.Timeout,
		regexRules:     cfg.RegexRules,
		jsonFieldRules: cfg.JSONFieldRules,
		enableOTEL:     cfg.EnableOTEL,
		rotation:       cfg.Rotation,
	}
	l.minLevel.Store(int32(cfg.MinLevel))
	l.dynConfig.MinLevel = cfg.MinLevel
	l.dynConfig.RegexRules = cfg.RegexRules
	l.dynConfig.JSONFieldRules = cfg.JSONFieldRules
	l.dynConfig.Retry = cfg.Retry
	l.dynConfig.Hooks = cfg.Hooks
	l.dynConfig.Batch = cfg.Batch

	// Khởi tạo rotation writer nếu bật
	if cfg.Rotation.Enable {
		if w := initRotationWriter(cfg.Rotation); w != nil {
			l.rotationSink = &writerSink{
				Name:   "rotation",
				Writer: w,
				Closer: w.(io.Closer),
			}
		}
	}

	l.startWorkers()
	if l.hookAsync {
		l.startHookRunner()
	}
	return l
}

// startWorkers khởi động worker xử lý log.
func (l *Logger) startWorkers() {
	for i := 0; i < l.workers; i++ {
		l.wg.Add(1)
		go l.workerLoop()
	}
}

// ensureInit đảm bảo globalLogger đã được khởi tạo.
func ensureInit() {
	if globalLogger == nil {
		InitLogger(INFO, "UTC")
	}
}

// Copyright (c) 2025 Nguyễn Thanh Phương
// This source code is licensed under the MIT License found in the LICENSE file.

// Package unologger - init.go
// Khởi tạo logger mặc định (globalLogger) và cung cấp hàm tạo logger riêng (detached logger).

package unologger

import (
	"io"
	"os"
	"strconv"
	"sync"
	"time"
)

// InitLogger khởi tạo global logger với minLevel và timezone.
// Nếu timezone không hợp lệ sẽ dùng UTC.
func InitLogger(minLevel Level, timezone string) {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		loc = time.UTC
	}
	l := &Logger{
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
		hookErrMax:  defaultHookErrMax,
	}
	l.minLevel.Store(int32(minLevel))
	l.dynConfig.MinLevel = minLevel
	// NEW: khởi tạo atomic batch và cờ JSON mặc định
	l.batchSizeA.Store(1)
	l.batchWaitA.Store(int64(time.Second))
	l.jsonFmtFlag.Store(false)

	globalMu.Lock()
	globalLogger = l
	globalMu.Unlock()

	l.startWorkers()
}

// helper: build extra writer sinks from config
func buildExtraSinks(ws []io.Writer, names []string) []writerSink {
	var sinks []writerSink
	for i, w := range ws {
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
		sinks = append(sinks, s)
	}
	return sinks
}

// helper: create logger from config (shared by init/reinit/detached)
func newLoggerFromConfig(cfg Config) *Logger {
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
	if len(cfg.RegexPatternMap) > 0 {
		cfg.RegexRules = append(cfg.RegexRules, compileMaskRegexes(cfg.RegexPatternMap)...)
	}
	// Clamp các giá trị cấu hình để an toàn
	if cfg.Buffer <= 0 {
		cfg.Buffer = 1024
	}
	if cfg.Workers <= 0 {
		cfg.Workers = 1
	}
	if cfg.Batch.Size <= 0 {
		cfg.Batch.Size = 1
	}
	if cfg.Batch.MaxWait <= 0 {
		cfg.Batch.MaxWait = time.Second
	}
	if cfg.Hook.Workers <= 0 {
		cfg.Hook.Workers = 1
	}
	if cfg.Hook.Queue <= 0 {
		cfg.Hook.Queue = 1024
	}

	var formatter Formatter
	if cfg.Formatter != nil {
		formatter = cfg.Formatter
	} else if cfg.JSON {
		formatter = &JSONFormatter{}
	} else {
		formatter = &TextFormatter{}
	}

	l := &Logger{
		stdOut:         cfg.Stdout,
		errOut:         cfg.Stderr,
		loc:            loc,
		jsonFmt:        cfg.JSON,
		formatter:      formatter, // NEW: Gán formatter
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
		rotation:       cfg.Rotation,
		hookErrMax:     defaultHookErrMax,
	}
	// Áp dụng cờ JSON và OTEL an toàn
	l.jsonFmtFlag.Store(cfg.JSON)
	l.enableOTEL.Store(cfg.EnableOTEL)

	l.batchSizeA.Store(int64(cfg.Batch.Size))    // NEW
	l.batchWaitA.Store(int64(cfg.Batch.MaxWait)) // NEW (ns)

	l.extraW = buildExtraSinks(cfg.Writers, cfg.WriterNames)
	l.minLevel.Store(int32(cfg.MinLevel))
	l.dynConfig.MinLevel = cfg.MinLevel
	l.dynConfig.RegexRules = cfg.RegexRules
	l.dynConfig.JSONFieldRules = cfg.JSONFieldRules
	l.dynConfig.Retry = cfg.Retry
	l.dynConfig.Hooks = cfg.Hooks
	l.dynConfig.Batch = cfg.Batch

	if cfg.Rotation.Enable {
		if w := initRotationWriter(cfg.Rotation); w != nil {
			l.rotationSink = &writerSink{
				Name:   "rotation",
				Writer: w,
				Closer: w.(io.Closer),
			}
		}
	}
	return l
}

// InitLoggerWithConfig khởi tạo global logger từ Config.
func InitLoggerWithConfig(cfg Config) {
	l := newLoggerFromConfig(cfg)
	globalMu.Lock()
	globalLogger = l
	globalMu.Unlock()
	l.startWorkers()
	if l.hookAsync {
		l.startHookRunner()
	}
}

// NewDetachedLogger tạo logger độc lập, không ảnh hưởng global logger.
func NewDetachedLogger(cfg Config) *Logger {
	l := newLoggerFromConfig(cfg)
	l.startWorkers()
	if l.hookAsync {
		l.startHookRunner()
	}
	return l
}

// ReinitGlobalLogger thay thế global logger theo cfg và đóng logger cũ với timeout.
func ReinitGlobalLogger(cfg Config, closeOldTimeout time.Duration) (*Logger, error) {
	ensureInit()
	old := func() *Logger {
		globalMu.RLock()
		defer globalMu.RUnlock()
		return globalLogger
	}()
	newL := newLoggerFromConfig(cfg)
	newL.startWorkers()
	if newL.hookAsync {
		newL.startHookRunner()
	}
	globalMu.Lock()
	globalLogger = newL
	globalMu.Unlock()

	var err error
	if old != nil {
		err = closeLogger(old, closeOldTimeout)
	}
	return newL, err
}

// startWorkers khởi động worker xử lý log.
func (l *Logger) startWorkers() {
	for i := 0; i < l.workers; i++ {
		l.wg.Add(1)
		go l.workerLoop()
	}
}

// ensureInit đảm bảo globalLogger đã được khởi tạo.
var ensureInitOnce sync.Once

func ensureInit() {
	ensureInitOnce.Do(func() {
		globalMu.RLock()
		already := globalLogger != nil
		globalMu.RUnlock()
		if already {
			return
		}
		InitLogger(INFO, "UTC")
	})
}

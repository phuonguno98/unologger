// Copyright (c) 2025 Nguyễn Thanh Phương
// This source code is licensed under the MIT License found in the LICENSE file.

// Package unologger provides a flexible and feature-rich logging library for Go applications.
// This file handles the initialization of the global logger and provides functions
// for creating detached logger instances.

package unologger

import (
	"io"
	"os"
	"strconv"
	"sync"
	"time"
)

// InitLogger initializes the global logger with a specified minimum log level and timezone.
// If the timezone is invalid, UTC is used as a fallback.
func InitLogger(minLevel Level, timezone string) {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		loc = time.UTC
	}
	l := &Logger{
		stdOut:      os.Stdout,
		errOut:      os.Stderr,
		loc:         loc,
		jsonFmt:     false, // Legacy flag
		ch:          make(chan *logEntry, 1024),
		workers:     1,
		nonBlocking: false,
		dropOldest:  false,
		batchSize:   1,           // Legacy batch size
		batchWait:   time.Second, // Legacy batch wait
		retryPolicy: RetryPolicy{},
		minLevel:    atomicLevel{},
		hookErrMax:  defaultHookErrMax,
	}
	l.minLevel.Store(int32(minLevel))
	l.dynConfig.MinLevel = minLevel
	// Initialize atomic batch and JSON flags
	l.batchSizeA.Store(1)
	l.batchWaitA.Store(int64(time.Second))
	l.jsonFmtFlag.Store(false) // Corrected: pass bool directly

	globalMu.Lock()
	globalLogger = l
	globalMu.Unlock()

	l.startWorkers()
}

// buildExtraSinks is a helper function to create writerSinks from a list of io.Writer and their names.
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

// newLoggerFromConfig is a helper function to create a new Logger instance based on the provided Config.
// It applies default values for unset configuration fields and initializes internal components.
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
		cfg.RegexRules = append(cfg.RegexRules, compileMaskRegexes(cfg.RegexPatternMap)...) // Assuming compileMaskRegexes exists
	}
	// Clamp configuration values to safe defaults
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
		formatter = &JSONFormatter{} // Assuming JSONFormatter exists
	} else {
		formatter = &TextFormatter{} // Assuming TextFormatter exists
	}

	l := &Logger{
		stdOut:         cfg.Stdout,
		errOut:         cfg.Stderr,
		loc:            loc,
		jsonFmt:        cfg.JSON, // Legacy flag
		formatter:      formatter,
		ch:             make(chan *logEntry, cfg.Buffer),
		workers:        cfg.Workers,
		nonBlocking:    cfg.NonBlocking,
		dropOldest:     cfg.DropOldest,
		batchSize:      cfg.Batch.Size,    // Legacy batch size
		batchWait:      cfg.Batch.MaxWait, // Legacy batch wait
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
	// Apply JSON and OTEL flags atomically
	l.jsonFmtFlag.Store(cfg.JSON)      // Corrected: pass bool directly
	l.enableOTel.Store(cfg.EnableOTel) // Corrected: pass bool directly

	l.batchSizeA.Store(int64(cfg.Batch.Size))
	l.batchWaitA.Store(int64(cfg.Batch.MaxWait))

	l.extraW = buildExtraSinks(cfg.Writers, cfg.WriterNames)
	l.minLevel.Store(int32(cfg.MinLevel))
	l.dynConfig.MinLevel = cfg.MinLevel
	l.dynConfig.RegexRules = cfg.RegexRules
	l.dynConfig.JSONFieldRules = cfg.JSONFieldRules
	l.dynConfig.Retry = cfg.Retry
	l.dynConfig.Hooks = cfg.Hooks
	l.dynConfig.Batch = cfg.Batch

	if cfg.Rotation.Enable {
		if w := initRotationWriter(cfg.Rotation); w != nil { // Assuming initRotationWriter exists
			l.rotationSink = &writerSink{
				Name:   "rotation",
				Writer: w,
				Closer: w.(io.Closer),
			}
		}
	}
	return l
}

// InitLoggerWithConfig initializes the global logger using a comprehensive Config struct.
func InitLoggerWithConfig(cfg Config) {
	l := newLoggerFromConfig(cfg)
	globalMu.Lock()
	globalLogger = l
	globalMu.Unlock()
	l.startWorkers()
	if l.hookAsync {
		l.startHookRunner() // Assuming startHookRunner exists
	}
}

// NewDetachedLogger creates and returns a new independent Logger instance.
// This logger operates separately from the global logger.
func NewDetachedLogger(cfg Config) *Logger {
	l := newLoggerFromConfig(cfg)
	l.startWorkers()
	if l.hookAsync {
		l.startHookRunner() // Assuming startHookRunner exists
	}
	return l
}

// ReinitGlobalLogger replaces the current global logger with a new one based on the provided Config.
// It attempts to gracefully close the old logger within the specified timeout.
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
		newL.startHookRunner() // Assuming startHookRunner exists
	}
	globalMu.Lock()
	globalLogger = newL
	globalMu.Unlock()

	var err error
	if old != nil {
		err = closeLogger(old, closeOldTimeout) // Assuming closeLogger exists
	}
	return newL, err
}

// startWorkers starts the goroutines responsible for processing log entries from the internal channel.
func (l *Logger) startWorkers() {
	for i := 0; i < l.workers; i++ {
		l.wg.Add(1)
		go l.workerLoop() // Assuming l.workerLoop exists
	}
}

// ensureInit ensures that the globalLogger has been initialized.
// If not, it initializes it with default INFO level and UTC timezone.
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

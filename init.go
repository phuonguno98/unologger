// Copyright (c) 2025 Nguyễn Thanh Phương
// This source code is licensed under the MIT License found in the LICENSE file.

// Package unologger provides a flexible and feature-rich logging library for Go applications.
// This file handles the initialization of the global logger and the creation of
// new, independent logger instances.

package unologger

import (
	"io"
	"os"
	"strconv"
	"sync"
	"time"
)

var (
	globalLogger   *Logger
	globalMu       sync.RWMutex
	ensureInitOnce sync.Once
)

// InitLogger initializes the global logger with a minimum log level and timezone.
// It uses default settings for other options like output format and batching.
//
// Deprecated: This function is provided for basic setup and backward compatibility.
// For full control over the logger's configuration, use InitLoggerWithConfig.
func InitLogger(minLevel Level, timezone string) {
	cfg := Config{
		MinLevel: minLevel,
		Timezone: timezone,
		// Set other fields to their default values explicitly for clarity.
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
		JSON:        false,
		Buffer:      1024,
		Workers:     1,
		NonBlocking: false,
		DropOldest:  false,
		Batch:       BatchConfig{Size: 1, MaxWait: time.Second},
		Hook:        HookConfig{Workers: 1, Queue: 1024},
	}
	// Create and set the global logger from the constructed config.
	l := newLoggerFromConfig(cfg)
	globalMu.Lock()
	globalLogger = l
	globalMu.Unlock()
	l.start()
}

// InitLoggerWithConfig initializes the global logger using the provided Config struct.
// This is the recommended way to initialize the logger, as it provides full control
// over all features like batching, rotation, hooks, and masking.
func InitLoggerWithConfig(cfg Config) {
	l := newLoggerFromConfig(cfg)
	globalMu.Lock()
	globalLogger = l
	globalMu.Unlock()
	l.start()
}

// NewDetachedLogger creates and returns a new, independent Logger instance from the
// provided configuration. This logger does not affect the global logger and is useful
// for creating separate logging contexts, such as for specific libraries or temporary tasks.
func NewDetachedLogger(cfg Config) *Logger {
	l := newLoggerFromConfig(cfg)
	l.start()
	return l
}

// ReinitGlobalLogger safely replaces the current global logger with a new one.
// It first creates and starts the new logger, then atomically swaps it with the old one.
// Finally, it attempts to gracefully close the old logger within the given timeout.
// This is useful for applying a completely new configuration at runtime without downtime.
func ReinitGlobalLogger(cfg Config, closeOldTimeout time.Duration) (*Logger, error) {
	ensureInit()
	oldLogger := GlobalLogger()

	// Create and start the new logger before acquiring the lock.
	newLogger := newLoggerFromConfig(cfg)
	newLogger.start()

	globalMu.Lock()
	globalLogger = newLogger
	globalMu.Unlock()

	var err error
	if oldLogger != nil {
		err = closeLogger(oldLogger, closeOldTimeout)
	}
	return newLogger, err
}

// newLoggerFromConfig is the core factory function for creating a Logger instance.
// It takes a user-provided Config, applies sane defaults and validation,
// and initializes all internal components of the logger.
func newLoggerFromConfig(cfg Config) *Logger {
	// --- Apply Defaults ---
	if cfg.Stdout == nil {
		cfg.Stdout = os.Stdout
	}
	if cfg.Stderr == nil {
		cfg.Stderr = os.Stderr
	}
	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil || loc == nil {
		loc = time.UTC
	}
	if len(cfg.RegexPatternMap) > 0 {
		cfg.RegexRules = append(cfg.RegexRules, compileMaskRegexes(cfg.RegexPatternMap)...)
	}

	// --- Clamp Values to Safe Ranges ---
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

	// --- Select Formatter ---
	var formatter Formatter
	if cfg.Formatter != nil {
		formatter = cfg.Formatter
	} else if cfg.JSON {
		formatter = &JSONFormatter{}
	} else {
		formatter = &TextFormatter{}
	}

	

	// --- Create Logger Instance ---
	l := &Logger{
		stdOut:         cfg.Stdout,
		errOut:         cfg.Stderr,
		loc:            loc,
		formatter:      formatter,
		ch:             make(chan *logEntry, cfg.Buffer),
		workers:        cfg.Workers,
		nonBlocking:    cfg.NonBlocking,
		dropOldest:     cfg.DropOldest,
		retryPolicy:    cfg.Retry,
		hooks:          cfg.Hooks,
		hookAsync:      cfg.Hook.Async,
		hookWorkers:    cfg.Hook.Workers,
		hookQueue:      cfg.Hook.Queue,
		hookTimeout:    cfg.Hook.Timeout,
		regexRules:     cfg.RegexRules,
		jsonFieldRules: cfg.JSONFieldRules,
		hookErrMax:     defaultHookErrMax,
	}

	// --- Initialize Atomic and Dynamic Config ---
	l.minLevel.Store(int32(cfg.MinLevel))
	l.jsonFmtFlag.Store(cfg.JSON)
	l.enableOTel.Store(cfg.EnableOTel)
	l.batchSizeA.Store(int64(cfg.Batch.Size))
	l.batchWaitA.Store(int64(cfg.Batch.MaxWait))

	// Initialize dynamic config for runtime changes.
	l.dynConfig.MinLevel = cfg.MinLevel
	l.dynConfig.RegexRules = cfg.RegexRules
	l.dynConfig.JSONFieldRules = cfg.JSONFieldRules
	l.dynConfig.Retry = cfg.Retry
	l.dynConfig.Hooks = cfg.Hooks
	l.dynConfig.Batch = cfg.Batch

	// --- Initialize Writers ---
	l.extraW = buildExtraSinks(cfg.Writers, cfg.WriterNames)
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

// buildExtraSinks is a helper to convert slices of io.Writer and names into
// the internal writerSink struct.
func buildExtraSinks(ws []io.Writer, names []string) []writerSink {
	if len(ws) == 0 {
		return nil
	}
	sinks := make([]writerSink, 0, len(ws))
	for i, w := range ws {
		if w == nil {
			continue
		}
		name := "extra" + strconv.Itoa(i)
		if i < len(names) && names[i] != "" {
			name = names[i]
		}
		s := writerSink{Name: name, Writer: w}
		if c, ok := w.(io.Closer); ok {
			s.Closer = c
		}
		sinks = append(sinks, s)
	}
	return sinks
}

// start begins the logger's background processing goroutines (workers and hooks).
func (l *Logger) start() {
	l.startWorkers()
	if l.hookAsync {
		l.startHookRunner()
	}
}

// startWorkers launches the worker goroutines that process and write log entries.
func (l *Logger) startWorkers() {
	for i := 0; i < l.workers; i++ {
		l.wg.Add(1)
		go l.workerLoop()
	}
}

// ensureInit guarantees that the global logger is initialized, preventing nil panics.
// If the logger has not been initialized via InitLogger or InitLoggerWithConfig,
// this function will initialize it once with default settings (INFO level, UTC timezone).
// This allows the logger to work out-of-the-box with zero configuration.
func ensureInit() {
	ensureInitOnce.Do(func() {
		globalMu.RLock()
		alreadyInitialized := globalLogger != nil
		globalMu.RUnlock()
		if alreadyInitialized {
			return
		}
		// If no logger is configured, initialize with basic defaults.
		InitLogger(INFO, "UTC")
	})
}
// Copyright (c) 2025 Nguyễn Thanh Phương
// This source code is licensed under the MIT License found in the LICENSE file.

// Package unologger provides a flexible and feature-rich logging library for Go applications.
// This file defines the core data structures, types, and interfaces used throughout the library,
// including the main Config struct for initialization.

package unologger

import (
	"context"
	"io"
	"regexp"
	"sync"
	"sync/atomic"
	"time"
)

// Level represents the severity of a log entry.
// The zero value for Level is DEBUG.
type Level int32

// Log level constants.
const (
	// DEBUG level is for detailed information, typically of interest only when diagnosing problems.
	DEBUG Level = iota
	// INFO level is for informational messages that highlight the progress of the application.
	INFO
	// WARN level is for potentially harmful situations or events that are not errors.
	WARN
	// ERROR level is for error events that might still allow the application to continue running.
	ERROR
	// FATAL level is for severe error events that will presumably lead the application to abort.
	FATAL
)

// String returns the uppercase string representation of the log level.
func (lvl Level) String() string {
	switch lvl {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	case FATAL:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// Formatter defines the interface for converting a log event into a byte slice for output.
// This allows for custom log formats.
type Formatter interface {
	Format(ev HookEvent) ([]byte, error)
}

// RetryPolicy configures the retry behavior for transient errors during log writes.
type RetryPolicy struct {
	// MaxRetries is the maximum number of times to retry a failed write.
	// If 0, no retries will be attempted. Defaults to 0.
	MaxRetries int
	// Backoff is the base duration to wait before the first retry.
	// Defaults to 0.
	Backoff time.Duration
	// Jitter adds a random duration up to this value to the backoff, preventing thundering herd issues.
	// Defaults to 0.
	Jitter time.Duration
	// Exponential, if true, doubles the backoff duration after each failed retry.
	// Defaults to false.
	Exponential bool
}

// HookConfig configures the behavior of the hook execution system.
type HookConfig struct {
	// Async, if true, causes hooks to be executed asynchronously in a separate worker pool.
	// If false, hooks are executed synchronously on the same goroutine that calls the log method.
	// Defaults to false.
	Async bool
	// Workers is the number of goroutines in the pool for processing async hooks.
	// Only applies if Async is true. Defaults to 1.
	Workers int
	// Queue is the buffer size for the asynchronous hook event channel.
	// Only applies if Async is true. Defaults to 1024.
	Queue int
	// Timeout is the maximum duration to wait for a single hook to execute.
	// If a hook exceeds this timeout, it is abandoned, and an error is logged.
	// If 0, there is no timeout. Defaults to 0.
	Timeout time.Duration
}

// BatchConfig configures log batching to improve I/O performance.
// Batching groups multiple log entries together before writing them.
type BatchConfig struct {
	// Size is the maximum number of log entries to include in a single batch.
	// When a batch reaches this size, it is flushed. Defaults to 1 (no batching).
	Size int
	// MaxWait is the maximum time to wait before flushing a batch, even if it's not full.
	// This ensures logs are not held in memory for too long during periods of low activity.
	// Defaults to 1 second.
	MaxWait time.Duration
}

// MaskRuleRegex defines a single regex-based masking rule.
type MaskRuleRegex struct {
	Pattern     *regexp.Regexp // The compiled regular expression to match.
	Replacement string         // The string to replace matched content with.
}

// MaskFieldRule defines a rule for masking specific fields in structured (JSON) logs.
type MaskFieldRule struct {
	Keys        []string // The list of JSON field keys to mask (e.g., "password", "credit_card").
	Replacement string   // The string that will replace the original field's value.
}

// RotationConfig configures log file rotation using the lumberjack library.
type RotationConfig struct {
	// Enable turns log rotation on or off. If true, logs will be written to a rotating file.
	Enable bool
	// Filename is the path to the log file. Required if rotation is enabled.
	Filename string
	// MaxSizeMB is the maximum size in megabytes a log file can reach before it is rotated.
	MaxSizeMB int
	// MaxAge is the maximum number of days to retain old log files.
	MaxAge int
	// MaxBackups is the maximum number of old log files to keep.
	MaxBackups int
	// Compress determines if rotated log files should be compressed using gzip.
	Compress bool
}

// Config is the central configuration struct for creating a new Logger instance.
// It is passed to InitLoggerWithConfig or NewDetachedLogger.
type Config struct {
	// MinLevel is the minimum level of logs to process. Logs below this level are discarded.
	// Defaults to INFO.
	MinLevel Level
	// Timezone is the IANA Time Zone name for timestamps (e.g., "UTC", "America/New_York").
	// Defaults to "UTC" if empty or invalid.
	Timezone string
	// JSON, if true, sets the default formatter to JSONFormatter for structured logging.
	// This is ignored if a custom Formatter is provided. Defaults to false (plain text).
	JSON bool
	// Formatter specifies a custom log formatter. If set, it overrides the JSON flag.
	// Defaults to nil, which enables the standard TextFormatter or JSONFormatter.
	Formatter Formatter
	// Buffer is the size of the internal channel for queuing log entries.
	// A larger buffer can absorb logging spikes but uses more memory.
	// Defaults to 1024.
	Buffer int
	// Workers is the number of goroutines processing log entries from the buffer.
	// More workers can increase throughput on multi-core systems.
	// Defaults to 1.
	Workers int
	// NonBlocking, if true, prevents log calls from blocking when the buffer is full.
	// Instead, the log entry is dropped. See also DropOldest.
	NonBlocking bool
	// DropOldest, if true and NonBlocking is also true, drops the oldest entry from the
	// buffer to make room for the new one. If false, the new entry is dropped.
	// This has no effect if NonBlocking is false.
	DropOldest bool
	// Batch configures log batching. Defaults to disabled (size 1).
	Batch BatchConfig
	// Stdout is the writer for INFO and DEBUG logs. Defaults to os.Stdout.
	Stdout io.Writer
	// Stderr is the writer for WARN, ERROR, and FATAL logs. Defaults to os.Stderr.
	Stderr io.Writer
	// Writers is a slice of additional writers to send all logs to.
	Writers []io.Writer
	// WriterNames provides optional names for the additional writers, used for error stats.
	WriterNames []string
	// Retry configures the retry policy for failed writes. Defaults to disabled.
	Retry RetryPolicy
	// Hooks is a slice of functions to be executed for each log entry.
	Hooks []HookFunc
	// Hook configures the hook execution system (async, timeouts, etc.).
	Hook HookConfig
	// RegexRules is a slice of pre-compiled regex masking rules.
	RegexRules []MaskRuleRegex
	// RegexPatternMap is a map of regex patterns to their replacements for easy configuration.
	// These are compiled into RegexRules during initialization.
	RegexPatternMap map[string]string
	// JSONFieldRules defines rules for masking specific fields in JSON logs.
	JSONFieldRules []MaskFieldRule
	// Rotation configures log file rotation. Disabled by default.
	Rotation RotationConfig
	// EnableOTel, if true, enables automatic extraction of Trace and Span IDs from OpenTelemetry contexts.
	EnableOTel bool
}

// Fields is a map for adding structured, key-value data to a log entry.
type Fields map[string]interface{}

// HookEvent contains all the data associated with a single log event,
// passed to each hook function.
type HookEvent struct {
	Time     time.Time // The timestamp when the log event was created.
	Level    Level     // The severity level of the log.
	Module   string    // The module associated with the log via context.
	Message  string    // The final, formatted log message.
	TraceID  string    // OpenTelemetry Trace ID, if available.
	FlowID   string    // Custom Flow ID, if available.
	Attrs    Fields    // Key-value attributes from the context.
	Fields   Fields    // Key-value fields passed directly to the log call.
	JSONMode bool      // True if the logger is currently in JSON output mode.
}

// HookError stores detailed information about a hook execution that failed.
type HookError struct {
	Time    time.Time // The time when the hook error occurred.
	Level   Level     // The level of the original log entry.
	Module  string    // The module of the original log entry.
	Message string    // The message of the original log entry.
	Err     error     // The error returned by the hook, or a timeout/panic error.
}

// HookFunc defines the signature for a function that can be used as a hook.
// It receives a HookEvent and returns an error if it fails.
type HookFunc func(e HookEvent) error

// --- Internal Types ---

// ctxKey is a private type to prevent context key collisions.
type ctxKey struct{}

var (
	// ctxLoggerKey is the context key for storing a specific *Logger instance.
	ctxLoggerKey = ctxKey{}
	// ctxModuleKey is the context key for storing the module name.
	ctxModuleKey = ctxKey{}
	// ctxTraceIDKey is the context key for storing the trace ID.
	ctxTraceIDKey = ctxKey{}
	// ctxFlowIDKey is the context key for storing the flow ID.
	ctxFlowIDKey = ctxKey{}
	// ctxFieldsKey is the context key for storing contextual attributes (Fields).
	ctxFieldsKey = ctxKey{}
)

// hookTask is an internal wrapper for passing a hook event to the async worker pool.
type hookTask struct {
	event HookEvent
}

// writerSink is an internal struct that pairs an io.Writer with a name and an optional io.Closer.
type writerSink struct {
	Name   string
	Writer io.Writer
	Closer io.Closer
}

// Logger is the central struct of the library, managing the entire logging pipeline.
// It should be created via InitLoggerWithConfig or NewDetachedLogger.
type Logger struct {
	// --- Pipeline & Workers ---
	ch          chan *logEntry // The central channel for incoming log entries.
	workers     int            // Number of worker goroutines processing the channel.
	wg          sync.WaitGroup // Waits for workers to finish during shutdown.
	closed      atomicBool     // Indicates if the logger is shutting down.
	nonBlocking bool           // If true, enqueue operations don't block when `ch` is full.
	dropOldest  bool           // If true and non-blocking, drops the oldest entry from `ch`.

	// --- Output & Formatting ---
	stdOut      io.Writer      // Destination for non-error logs.
	errOut      io.Writer      // Destination for ERROR and FATAL logs.
	extraW      []writerSink   // Additional output destinations.
	rotationSink *writerSink    // A special writer for log rotation.
	outputsMu   sync.RWMutex   // Guards access to all output writers.
	formatter   Formatter      // Formats a log entry into bytes.
	loc         *time.Location // Timezone for timestamps.
	locMu       sync.RWMutex   // Guards access to the timezone location.
	jsonFmtFlag atomicBool     // Atomic flag for runtime JSON format toggling.
	formatterMu sync.RWMutex   // Guards access to the formatter.

	// --- Batching ---
	batchSizeA atomicI64 // Atomic batch size for lock-free reads.
	batchWaitA atomicI64 // Atomic batch wait duration (ns) for lock-free reads.

	// --- Masking ---
	regexRules     []MaskRuleRegex // Compiled regex rules for masking.
	jsonFieldRules []MaskFieldRule // Rules for masking specific JSON fields.

	// --- Hooks ---
	hooks       []HookFunc     // The slice of registered hook functions.
	hooksMu     sync.RWMutex   // Guards access to the hooks slice.
	hookAsync   bool           // If true, hooks are processed asynchronously.
	hookWorkers int            // Number of goroutines in the hook worker pool.
	hookQueue   int            // Buffer size for the async hook channel.
	hookTimeout time.Duration  // Timeout for a single hook execution.
	hookQueueCh chan hookTask  // The channel for async hook processing.
	hookWg      sync.WaitGroup // Waits for hook workers to finish during shutdown.
	hookErrLog  []HookError    // A circular buffer of recent hook errors.
	hookErrMu   sync.Mutex     // Guards access to hookErrLog.
	hookErrMax  int            // Max size of the hookErrLog buffer.

	// --- Telemetry & Dynamic Config ---
	enableOTel atomicBool    // Atomic flag to enable/disable OpenTelemetry integration.
	minLevel   atomicLevel   // Atomic minimum log level.
	dynConfig  DynamicConfig // Holds configuration that can be changed at runtime.

	// --- Statistics ---
	retryPolicy   RetryPolicy // The retry policy for failed writes.
	writtenCount  atomicI64   // Total log entries successfully written.
	droppedCount  atomicI64   // Total log entries dropped.
	batchCount    atomicI64   // Total batches processed.
	writeErrCount atomicI64   // Total errors encountered during writes.
	hookErrCount  atomicI64   // Total errors encountered during hook execution.
	writerErrs    sync.Map    // Stores error counts for specific writers.
}

// LoggerWithCtx is a lightweight wrapper that binds a *Logger instance to a context.Context.
// This is the primary way to use the logger after initialization, as it allows for the
// propagation of contextual metadata like module names and trace IDs.
type LoggerWithCtx struct {
	l   *Logger
	ctx context.Context
}

// logEntry is an internal representation of a single log event.
// These objects are pooled using a sync.Pool to reduce memory allocations.
type logEntry struct {
	lvl    Level
	ctx    context.Context
	t      time.Time
	tmpl   string
	args   []any
	fields Fields
}

// logBatch is an internal representation of a batch of log entries.
// These objects are pooled to reduce memory allocations.
type logBatch struct {
	items   []*logEntry
	created time.Time
}

// DynamicConfig holds the parts of the logger's configuration that can be safely
// modified at runtime. Access is protected by an internal mutex.
type DynamicConfig struct {
	mu             sync.RWMutex
	MinLevel       Level
	RegexRules     []MaskRuleRegex
	JSONFieldRules []MaskFieldRule
	Retry          RetryPolicy
	Hooks          []HookFunc
	Batch          BatchConfig
}

// --- Atomic Wrappers ---

// atomicLevel provides atomic operations for the Level type (int32).
type atomicLevel struct{ v int32 }

func (a *atomicLevel) Load() int32      { return atomic.LoadInt32(&a.v) }
func (a *atomicLevel) Store(val int32) { atomic.StoreInt32(&a.v, val) }

// atomicBool provides atomic operations for a boolean.
type atomicBool struct{ v uint32 }

func (a *atomicBool) Load() bool         { return atomic.LoadUint32(&a.v) != 0 }
func (a *atomicBool) Store(val bool)     { atomic.StoreUint32(&a.v, b32(val)) }
func (a *atomicBool) TrySetTrue() bool   { return atomic.CompareAndSwapUint32(&a.v, 0, 1) }

// atomicI64 provides atomic operations for an int64.
type atomicI64 struct{ v int64 }

func (a *atomicI64) Add(delta int64) { atomic.AddInt64(&a.v, delta) }
func (a *atomicI64) Load() int64      { return atomic.LoadInt64(&a.v) }
func (a *atomicI64) Store(val int64) { atomic.StoreInt64(&a.v, val) }

// b32 converts a boolean to a uint32 (0 or 1).
func b32(b bool) uint32 {
	if b {
		return 1
	}
	return 0
}

// --- Pools ---

var (
	// poolEntry reuses logEntry objects to reduce pressure on the garbage collector.
	poolEntry = sync.Pool{
		New: func() any { return &logEntry{} },
	}
	// poolBatch reuses logBatch objects.
	poolBatch = sync.Pool{
		New: func() any { return &logBatch{items: make([]*logEntry, 0, 64)} },
	}
)

const defaultHookErrMax = 1000

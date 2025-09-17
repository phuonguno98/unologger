// Copyright (c) 2025 Nguyễn Thanh Phương
// This source code is licensed under the MIT License found in the LICENSE file.

// Package unologger provides a flexible and feature-rich logging library for Go applications.
// It supports structured logging, dynamic configuration, data masking, log rotation,
// and OpenTelemetry integration.
package unologger

import (
	"context"
	"io"
	"regexp"
	"sync"
	"sync/atomic"
	"time"
)

// Level represents the severity level of a log entry.
// The order is: DEBUG < INFO < WARN < ERROR < FATAL.
type Level int32

// Log levels constants.
const (
	DEBUG Level = iota // Detailed information, typically of interest only when diagnosing problems.
	INFO               // Informational messages that highlight the progress of the application at coarse-grained level.
	WARN               // Potentially harmful situations.
	ERROR              // Error events that might still allow the application to continue running.
	FATAL              // Severe error events that will presumably lead the application to abort.
)

// String returns the string representation of the log level (e.g., "INFO").
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

// ctxKey is a custom type for context keys to avoid collisions.
type ctxKey string

// ctxLoggerKey is used to store a *Logger instance in the context.
type ctxLoggerKey struct{}

const (
	ctxModuleKey  ctxKey = "module"
	ctxTraceIDKey ctxKey = "trace_id"
	ctxFlowIDKey  ctxKey = "flow_id"
)

// Formatter is an interface for formatting a HookEvent into a byte slice.
type Formatter interface {
	Format(ev HookEvent) ([]byte, error)
}

// RetryPolicy configures retry behavior for failed log writes.
type RetryPolicy struct {
	MaxRetries  int           // Maximum number of retry attempts.
	Backoff     time.Duration // Base duration to wait between retries.
	Jitter      time.Duration // Random jitter added to the backoff duration.
	Exponential bool          // If true, backoff duration increases exponentially.
}

// HookConfig configures the asynchronous hook system.
type HookConfig struct {
	Async   bool          // Whether hooks should run asynchronously.
	Workers int           // Number of worker goroutines processing hooks.
	Queue   int           // Size of the hook event queue.
	Timeout time.Duration // Timeout for each individual hook execution.
}

// BatchConfig configures log batching to reduce I/O operations.
type BatchConfig struct {
	Size    int           // Maximum number of log entries in a batch.
	MaxWait time.Duration // Maximum time to wait before flushing a batch, even if not full.
}

// MaskRuleRegex defines a regex-based masking rule.
type MaskRuleRegex struct {
	Pattern     *regexp.Regexp // The regular expression pattern to match.
	Replacement string         // The string to replace matched patterns with.
}

// MaskFieldRule defines a field-level masking rule for JSON logs.
type MaskFieldRule struct {
	Keys        []string // List of field names to mask.
	Replacement string   // The string to replace the field's value with.
}

// RotationConfig configures log file rotation.
type RotationConfig struct {
	Enable     bool   // Whether log rotation is enabled.
	Filename   string // The base filename for log files.
	MaxSizeMB  int    // Maximum size in megabytes before a log file is rotated.
	MaxAge     int    // Maximum number of days to retain old log files.
	MaxBackups int    // Maximum number of old log files to keep.
	Compress   bool   // Whether to compress rotated log files.
}

// Config holds the configuration for initializing a Logger.
type Config struct {
	MinLevel        Level             // Minimum log level to process.
	Timezone        string            // Timezone for log timestamps (e.g., "Asia/Ho_Chi_Minh", "UTC").
	JSON            bool              // If true, logs are formatted as JSON; otherwise, plain text.
	Formatter       Formatter         // Custom formatter to use. If nil, JSON or Text formatter is used based on JSON flag.
	Buffer          int               // Size of the internal channel buffer for log entries.
	Workers         int               // Number of worker goroutines processing log entries.
	NonBlocking     bool              // If true, log enqueue operations are non-blocking.
	DropOldest      bool              // If non-blocking and buffer is full, drop the oldest log entry.
	Batch           BatchConfig       // Batching configuration.
	Stdout          io.Writer         // Writer for standard output logs (e.g., os.Stdout).
	Stderr          io.Writer         // Writer for error output logs (e.g., os.Stderr).
	Writers         []io.Writer       // Additional writers for log output.
	WriterNames     []string          // Corresponding names for additional writers.
	Retry           RetryPolicy       // Retry policy for write operations.
	Hooks           []HookFunc        // List of hook functions to execute.
	Hook            HookConfig        // Hook system configuration.
	RegexRules      []MaskRuleRegex   // Regex-based masking rules.
	RegexPatternMap map[string]string // Map of regex patterns to replacement strings for masking.
	JSONFieldRules  []MaskFieldRule   // Field-level masking rules for JSON logs.
	Rotation        RotationConfig    // Log file rotation configuration.
	EnableOTEL      bool              // Whether OpenTelemetry integration is enabled.
}

// Fields is a map for custom key-value pairs in log entries.
type Fields map[string]interface{}

// HookEvent contains data passed to hook functions.
type HookEvent struct {
	Time     time.Time         // Timestamp of the log entry.
	Level    Level             // Log level (DEBUG, INFO, WARN, ERROR, FATAL).
	Module   string            // Module name associated with the log.
	Message  string            // Formatted log message.
	TraceID  string            // OpenTelemetry Trace ID.
	FlowID   string            // Custom Flow ID.
	Attrs    map[string]string // Additional attributes.
	Fields   Fields            // Custom key-value fields from the log entry.
	JSONMode bool              // True if the log is being formatted as JSON.
}

// HookError stores detailed information about a hook execution error.
type HookError struct {
	Time    time.Time // Time when the hook error occurred.
	Level   Level     // Log level of the original entry that triggered the hook.
	Module  string    // Module name from the original log entry.
	Message string    // Message from the original log entry.
	Err     error     // The error returned by the hook function.
}

// HookFunc is the signature for a hook function.
type HookFunc func(e HookEvent) error

// hookTask represents an asynchronous hook job.
type hookTask struct {
	event HookEvent
}

// writerSink wraps an io.Writer with a name and an optional io.Closer.
type writerSink struct {
	Name   string
	Writer io.Writer
	Closer io.Closer
}

// Logger manages the entire logging pipeline, including hooks, masking, outputs, and statistics.
type Logger struct {
	stdOut io.Writer // Standard output writer.
	errOut io.Writer // Error output writer.
	extraW []writerSink // Additional writers.
	loc    *time.Location // Timezone location for timestamps.

	jsonFmt     bool       // Legacy JSON format flag, kept for internal compatibility.
	jsonFmtFlag atomicBool // Atomic flag for dynamic JSON format changes at runtime.

	outputsMu sync.RWMutex // Guards dynamic output changes.
	locMu     sync.RWMutex // Guards timezone location read/write.

	formatter Formatter // The log formatter (e.g., JSON, Text, custom).

	ch          chan *logEntry // Channel for enqueuing log entries.
	workers     int            // Number of worker goroutines processing log entries.
	nonBlocking bool           // If true, enqueue operations are non-blocking.
	dropOldest  bool           // If non-blocking and buffer is full, drop the oldest entry.
	batchSize   int            // Legacy batch size.
	batchWait   time.Duration  // Legacy batch wait duration.

	batchSizeA atomicI64 // Atomic batch size for dynamic changes.
	batchWaitA atomicI64 // Atomic batch wait duration (in nanoseconds) for dynamic changes.

	retryPolicy RetryPolicy // Retry policy for write operations.

	regexRules     []MaskRuleRegex // Regex-based masking rules.
	jsonFieldRules []MaskFieldRule // Field-level masking rules for JSON logs.

	hooks       []HookFunc    // List of hook functions.
	hookAsync   bool          // Whether hooks run asynchronously.
	hookWorkers int           // Number of worker goroutines for hooks.
	hookQueue   int           // Size of the hook event queue.
	hookTimeout time.Duration // Timeout for individual hook execution.
	hookQueueCh chan hookTask // Channel for enqueuing hook tasks.
	hookWg      sync.WaitGroup // WaitGroup for asynchronous hook workers.
	hookErrLog  []HookError   // Buffer to store recent hook errors.
	hookErrMu   sync.Mutex    // Guards access to hookErrLog.
	hooksMu     sync.RWMutex  // Guards access to l.hooks.
	hookErrMax  int           // Maximum number of hook errors to retain in hookErrLog.

	enableOTEL atomicBool // Atomic flag to enable/disable OpenTelemetry integration dynamically.

	rotation     RotationConfig // Log file rotation configuration.
	rotationSink *writerSink    // Internal writer for log rotation (if enabled).

	minLevel atomicLevel // Atomic minimum log level for dynamic changes.
	closed   atomicBool  // Atomic flag indicating if the logger is closed.
	wg       sync.WaitGroup // WaitGroup for log processing workers.

	writtenCount  atomicI64 // Counter for successfully written log entries.
	droppedCount  atomicI64 // Counter for dropped log entries (due to full buffer).
	batchCount    atomicI64 // Counter for processed batches.
	writeErrCount atomicI64 // Counter for errors during log writing.
	hookErrCount  atomicI64 // Counter for errors during hook execution.
	writerErrs    sync.Map  // Map to store specific writer errors.

	dynConfig DynamicConfig // Dynamic configuration settings.
}

// LoggerWithCtx is a wrapper that holds a *Logger and a context.Context
// to facilitate logging with associated metadata.
type LoggerWithCtx struct {
	l   *Logger
	ctx context.Context
}

// logEntry represents an internal log record before formatting.
type logEntry struct {
	lvl    Level         // Log level.
	ctx    context.Context // Context associated with the log.
	t      time.Time     // Timestamp of the log.
	tmpl   string        // Format string for the message.
	args   []any         // Arguments for the format string.
	fields Fields        // Custom key-value fields.
}

// logBatch groups multiple logEntry instances for batch processing.
type logBatch struct {
	items   []*logEntry
	created time.Time
}

// DynamicConfig allows runtime modification of logger settings.
// It is protected by a mutex for concurrent access.
type DynamicConfig struct {
	mu             sync.RWMutex    // Mutex to protect dynamic configuration fields.
	MinLevel       Level           // Current minimum log level.
	RegexRules     []MaskRuleRegex // Current regex-based masking rules.
	JSONFieldRules []MaskFieldRule // Current field-level masking rules for JSON logs.
	Retry          RetryPolicy     // Current retry policy.
	Hooks          []HookFunc      // Current list of hook functions.
	Batch          BatchConfig     // Current batching configuration.
}

// atomicLevel provides atomic operations for a Level type.
type atomicLevel struct{ v int32 }

// Load atomically loads the value of atomicLevel.
func (a *atomicLevel) Load() int32 { return atomic.LoadInt32(&a.v) }

// Store atomically stores the value into atomicLevel.
func (a *atomicLevel) Store(val int32) { atomic.StoreInt32(&a.v, val) }

// atomicBool provides atomic operations for a boolean value.
type atomicBool struct{ v uint32 }

// Load atomically loads the boolean value of atomicBool.
func (a *atomicBool) Load() bool { return atomic.LoadUint32(&a.v) != 0 }

// Store atomically stores the boolean value into atomicBool.
func (a *atomicBool) Store(val bool) {
	if val {
		atomic.StoreUint32(&a.v, 1)
	} else {
		atomic.StoreUint32(&a.v, 0)
	}
}

// SetTrue atomically sets the boolean value to true.
func (a *atomicBool) SetTrue() { atomic.StoreUint32(&a.v, 1) }

// IsTrue atomically checks if the boolean value is true.
func (a *atomicBool) IsTrue() bool { return atomic.LoadUint32(&a.v) != 0 }

// TrySetTrue attempts to atomically set the value to true if it's currently false.
// It returns true if the value was successfully changed to true.
func (a *atomicBool) TrySetTrue() bool {
	return atomic.CompareAndSwapUint32(&a.v, 0, 1)
}

// atomicI64 provides atomic operations for an int64 value.
type atomicI64 struct{ v int64 }

// Add atomically adds a delta to the int64 value.
func (a *atomicI64) Add(delta int64) { atomic.AddInt64(&a.v, delta) }

// Load atomically loads the int64 value of atomicI64.
func (a *atomicI64) Load() int64 { return atomic.LoadInt64(&a.v) }

// Store atomically stores the int64 value into atomicI64.
func (a *atomicI64) Store(val int64) { atomic.StoreInt64(&a.v, val) }

// poolEntry is a sync.Pool for reusing logEntry objects to reduce allocations.
var poolEntry = sync.Pool{
	New: func() any { return &logEntry{} },
}

// poolBatch is a sync.Pool for reusing logBatch objects to reduce allocations.
var poolBatch = sync.Pool{
	New: func() any { return &logBatch{items: make([]*logEntry, 0, 64)} },
}

// globalLogger is the default global Logger instance.
var globalLogger *Logger

// globalMu protects concurrent access to globalLogger.
var globalMu sync.RWMutex

// defaultHookErrMax is the default maximum number of hook errors to retain.
const defaultHookErrMax = 1000
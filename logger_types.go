// Copyright (c) 2025 Nguyễn Thanh Phương
// This source code is licensed under the MIT License found in the LICENSE file.

// Package unologger - logger_types.go
// File nền tảng: chỉ chứa KHAI BÁO hằng số, kiểu dữ liệu, struct cốt lõi, khóa context,
// wrapper atomic, và các biến toàn cục cần thiết để các file khác dựa vào.
// Mục tiêu: tránh chồng chéo code, đảm bảo các file tiếp theo biên dịch được khi lần lượt bổ sung logic.

package unologger

import (
	"context"
	"io"
	"regexp"
	"sync"
	"sync/atomic"
	"time"
)

//
// ===== Cấp độ log =====
//

// Level biểu diễn cấp độ log.
// Thứ tự: DEBUG < INFO < WARN < ERROR < FATAL.
type Level int32

// Các hằng số cấp độ log.
const (
	DEBUG Level = iota // Thông tin chi tiết phục vụ debug
	INFO               // Thông tin chung về tiến trình hoạt động
	WARN               // Cảnh báo bất thường nhưng chưa gây lỗi nghiêm trọng
	ERROR              // Lỗi nghiêm trọng cần xử lý
	FATAL              // Lỗi nghiêm trọng nhất; thường ghi log và kết thúc tiến trình
)

// String trả về tên cấp độ log (in hoa).
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

//
// ===== Khóa context =====
//

// Dùng kiểu riêng cho key để tránh xung đột package.
type ctxKey string

// ctxLoggerKey dùng dạng struct{} để gắn *Logger vào context.
type ctxLoggerKey struct{}

const (
	ctxModuleKey  ctxKey = "module"
	ctxTraceIDKey ctxKey = "trace_id"
	ctxFlowIDKey  ctxKey = "flow_id"
	ctxAttrsKey   ctxKey = "attrs"
)

//
// ===== Cấu hình & chính sách =====
//

// RetryPolicy cấu hình retry khi ghi log thất bại.
type RetryPolicy struct {
	MaxRetries  int           // Số lần thử lại tối đa
	Backoff     time.Duration // Thời gian chờ cơ sở giữa các lần thử
	Jitter      time.Duration // Nhiễu ngẫu nhiên cộng thêm vào Backoff
	Exponential bool          // Nếu true, backoff tăng theo cấp số nhân
}

// HookConfig cấu hình hệ thống hooks.
type HookConfig struct {
	Async   bool          // Chạy hook bất đồng bộ
	Workers int           // Số lượng worker hook
	Queue   int           // Kích thước hàng đợi hook
	Timeout time.Duration // Timeout cho mỗi hook
}

// BatchConfig cấu hình gom lô (batch) để giảm số lần I/O.
type BatchConfig struct {
	Size    int           // Số log tối đa trong một batch
	MaxWait time.Duration // Thời gian tối đa chờ trước khi flush batch
}

// MaskRuleRegex che mờ theo regex.
type MaskRuleRegex struct {
	Pattern     *regexp.Regexp // Mẫu regex
	Replacement string         // Chuỗi thay thế
}

// MaskFieldRule che mờ theo tên trường trong JSON log.
type MaskFieldRule struct {
	Keys        []string // Danh sách tên trường cần che
	Replacement string   // Chuỗi thay thế
}

// RotationConfig cấu hình xoay file log.
type RotationConfig struct {
	Enable     bool   // Bật/tắt rotation
	Filename   string // Đường dẫn file log
	MaxSizeMB  int    // Dung lượng tối đa (MB) trước khi xoay
	MaxAge     int    // Số ngày lưu file log cũ
	MaxBackups int    // Số file log cũ tối đa
	Compress   bool   // Nén file log cũ
}

// Config chứa cấu hình khởi tạo logger.
type Config struct {
	MinLevel        Level             // Cấp độ log tối thiểu
	Timezone        string            // Múi giờ
	JSON            bool              // Bật/tắt định dạng JSON
	Buffer          int               // Kích thước buffer channel log
	Workers         int               // Số lượng worker xử lý log
	NonBlocking     bool              // Chế độ non-blocking khi enqueue log
	DropOldest      bool              // Nếu non-blocking và đầy, drop log cũ
	Batch           BatchConfig       // Cấu hình batch
	Stdout          io.Writer         // Writer cho log thường
	Stderr          io.Writer         // Writer cho log lỗi
	Writers         []io.Writer       // Danh sách writer phụ
	WriterNames     []string          // Tên tương ứng cho writer phụ
	Retry           RetryPolicy       // Cấu hình retry
	Hooks           []HookFunc        // Danh sách hook
	Hook            HookConfig        // Cấu hình hook
	RegexRules      []MaskRuleRegex   // Quy tắc masking regex
	RegexPatternMap map[string]string // Map pattern string -> replacement
	JSONFieldRules  []MaskFieldRule   // Quy tắc masking field-level JSON
	Rotation        RotationConfig    // Cấu hình rotation
	EnableOTEL      bool              // Bật tích hợp OpenTelemetry
}

//
// ===== Hook types =====
//

// HookEvent là dữ liệu gửi vào hook.
// Fields là kiểu dữ liệu để truyền các cặp key-value tùy chỉnh vào log.
type Fields map[string]interface{}

// HookEvent là dữ liệu gửi vào hook.
type HookEvent struct {
	Time     time.Time         // Thời điểm log
	Level    Level             // Cấp độ log (DEBUG, INFO, WARN, ERROR, FATAL)
	Module   string            // Tên module
	Message  string            // Nội dung log
	TraceID  string            // Trace ID
	FlowID   string            // Flow ID
	Attrs    map[string]string // Thuộc tính bổ sung
	Fields   Fields            // Các trường dữ liệu tùy chỉnh
	JSONMode bool              // true nếu log ở định dạng JSON
}

// HookError lưu thông tin lỗi hook chi tiết.
type HookError struct {
	Time    time.Time // Thời điểm lỗi
	Level   Level     // Cấp độ log khi lỗi
	Module  string    // Module khi lỗi
	Message string    // Nội dung log khi lỗi
	Err     error     // Lỗi hook
}

// HookFunc là chữ ký hàm hook.
type HookFunc func(e HookEvent) error

// hookTask là công việc hook async (nội bộ).
type hookTask struct {
	event HookEvent
}

//
// ===== Kiểu dữ liệu cốt lõi =====
//

// writerSink bọc một io.Writer với tên và khả năng đóng (nếu có).
type writerSink struct {
	Name   string
	Writer io.Writer
	Closer io.Closer
}

// Logger quản lý toàn bộ pipeline ghi log, hooks, masking, outputs và thống kê.
type Logger struct {
	// Outputs
	stdOut io.Writer
	errOut io.Writer
	extraW []writerSink
	loc    *time.Location
	// Bật/tắt JSON: dùng atomic để tránh race khi runtime
	jsonFmt     bool       // legacy, giữ để tương thích nội bộ
	jsonFmtFlag atomicBool // NEW: cờ atomic, thay cho jsonFmt khi đọc/ghi runtime
	// NEW: guard dynamic output changes
	outputsMu sync.RWMutex
	// NEW: guard timezone location read/write
	locMu sync.RWMutex

	// Pipeline
	ch          chan *logEntry
	workers     int
	nonBlocking bool
	dropOldest  bool
	batchSize   int           // legacy
	batchWait   time.Duration // legacy
	// NEW: atomic batch config để tránh race khi runtime
	batchSizeA atomicI64
	batchWaitA atomicI64 // đơn vị: nanoseconds

	// Policies
	retryPolicy RetryPolicy

	// Masking
	regexRules     []MaskRuleRegex
	jsonFieldRules []MaskFieldRule

	// Hooks
	hooks       []HookFunc
	hookAsync   bool
	hookWorkers int
	hookQueue   int
	hookTimeout time.Duration
	hookQueueCh chan hookTask
	hookWg      sync.WaitGroup
	hookErrLog  []HookError
	hookErrMu   sync.Mutex
	// NEW: bảo vệ truy cập l.hooks
	hooksMu    sync.RWMutex
	hookErrMax int

	// OTEL interop
	enableOTEL atomicBool

	// Rotation
	rotation     RotationConfig
	rotationSink *writerSink // writer xoay file nội bộ (nếu bật)

	// State
	minLevel atomicLevel
	closed   atomicBool
	wg       sync.WaitGroup

	// Stats
	writtenCount  atomicI64
	droppedCount  atomicI64
	batchCount    atomicI64
	writeErrCount atomicI64
	hookErrCount  atomicI64
	writerErrs    sync.Map

	// Dynamic config (runtime)
	dynConfig DynamicConfig
}

// LoggerWithCtx là wrapper chứa *Logger và context để ghi log kèm metadata.
type LoggerWithCtx struct {
	l   *Logger
	ctx context.Context
}

// logEntry là bản ghi log nội bộ (chưa format).
type logEntry struct {
	lvl    Level
	ctx    context.Context
	t      time.Time
	tmpl   string
	args   []any
	fields Fields // NEW: Thêm trường Fields
}

// logBatch gom nhiều logEntry (nội bộ).
type logBatch struct {
	items   []*logEntry
	created time.Time
}

//
// ===== Cấu hình động (runtime) =====
//

// DynamicConfig cho phép thay đổi cấu hình logger khi runtime (được bảo vệ bằng mutex).
type DynamicConfig struct {
	mu             sync.RWMutex
	MinLevel       Level           // Cấp độ log tối thiểu hiện tại
	RegexRules     []MaskRuleRegex // Quy tắc masking regex
	JSONFieldRules []MaskFieldRule // Quy tắc masking field-level JSON
	Retry          RetryPolicy     // Cấu hình retry
	Hooks          []HookFunc      // Danh sách hook
	Batch          BatchConfig     // Cấu hình batch
}

//
// ===== Atomic wrappers =====
//

type atomicLevel struct{ v int32 }
type atomicBool struct{ v uint32 }
type atomicI64 struct{ v int64 }

// Load/Store cho atomicLevel.
func (a *atomicLevel) Load() int32     { return atomic.LoadInt32(&a.v) }
func (a *atomicLevel) Store(val int32) { atomic.StoreInt32(&a.v, val) }

// Load/Store/SetTrue/IsTrue cho atomicBool.
func (a *atomicBool) Load() bool { return atomic.LoadUint32(&a.v) != 0 }
func (a *atomicBool) Store(val bool) {
	if val {
		atomic.StoreUint32(&a.v, 1)
	} else {
		atomic.StoreUint32(&a.v, 0)
	}
}
func (a *atomicBool) SetTrue()     { atomic.StoreUint32(&a.v, 1) }
func (a *atomicBool) IsTrue() bool { return atomic.LoadUint32(&a.v) != 0 }

// NEW: TrySetTrue đặt true nếu hiện tại đang là false, trả về true nếu thành công.
func (a *atomicBool) TrySetTrue() bool {
	return atomic.CompareAndSwapUint32(&a.v, 0, 1)
}

// Add/Load/Store cho atomicI64.
func (a *atomicI64) Add(delta int64) { atomic.AddInt64(&a.v, delta) }
func (a *atomicI64) Load() int64     { return atomic.LoadInt64(&a.v) }
func (a *atomicI64) Store(val int64) { atomic.StoreInt64(&a.v, val) }

//
// ===== Pool nội bộ để giảm GC =====
//

// Pool cho logEntry.
var poolEntry = sync.Pool{
	New: func() any { return &logEntry{} },
}

// Pool cho batch.
var poolBatch = sync.Pool{
	New: func() any { return &logBatch{items: make([]*logEntry, 0, 64)} },
}

//
// ===== Biến toàn cục =====
//

// globalLogger là logger mặc định (nếu được init).
var globalLogger *Logger

// globalMu bảo vệ mọi thao tác đọc/ghi globalLogger để tránh race.
var globalMu sync.RWMutex

// NEW: giới hạn mặc định số bản ghi lỗi hook giữ lại
const defaultHookErrMax = 1000

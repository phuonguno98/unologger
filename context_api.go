// Copyright (c) 2025 Nguyễn Thanh Phương
// This source code is licensed under the MIT License found in the LICENSE file.

// Package unologger - context_api.go
// Cung cấp các hàm thao tác với Logger qua context.Context.
// Cho phép gắn/lấy Logger, module, trace_id, flow_id, attributes và ghi log trực tiếp từ LoggerWithCtx.

package unologger

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"time"
)

// WithLogger gắn một *Logger cụ thể vào context.
func WithLogger(ctx context.Context, l *Logger) context.Context {
	return context.WithValue(ctx, ctxLoggerKey{}, l)
}

// LoggerFromContext trả về *Logger đã gắn vào ctx (nếu có).
func LoggerFromContext(ctx context.Context) (*Logger, bool) {
	l, ok := ctx.Value(ctxLoggerKey{}).(*Logger)
	return l, ok
}

// WithModule gắn tên module vào context và trả về LoggerWithCtx mới.
func WithModule(ctx context.Context, module string) LoggerWithCtx {
	ensureInit()
	ctx = context.WithValue(ctx, ctxModuleKey, module)
	return GetLogger(ctx)
}

// WithTraceID gắn trace_id vào context. Nếu đã có trace_id thì ghi đè.
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, ctxTraceIDKey, traceID)
}

// WithFlowID gắn flow_id vào context.
func WithFlowID(ctx context.Context, flowID string) context.Context {
	return context.WithValue(ctx, ctxFlowIDKey, flowID)
}

// WithAttrs gắn thêm các thuộc tính bổ sung vào context.
// Nếu key trùng sẽ ghi đè giá trị mới.
func WithAttrs(ctx context.Context, attrs map[string]string) context.Context {
	if attrs == nil {
		return ctx
	}
	existing, _ := ctx.Value(ctxAttrsKey).(map[string]string)
	newMap := make(map[string]string, len(existing)+len(attrs))
	for k, v := range existing {
		newMap[k] = v
	}
	for k, v := range attrs {
		newMap[k] = v
	}
	return context.WithValue(ctx, ctxAttrsKey, newMap)
}

// EnsureTraceIDCtx đảm bảo context có trace_id, nếu chưa có sẽ tự sinh mới.
// Nếu globalLogger.enableOTEL=true và context có trace/span từ OpenTelemetry, sẽ ưu tiên dùng trace_id từ đó.
func EnsureTraceIDCtx(ctx context.Context) context.Context {
	if id, ok := ctx.Value(ctxTraceIDKey).(string); ok && id != "" {
		return ctx
	}
	if globalLogger != nil && globalLogger.enableOTEL {
		if tid := extractOTELTraceID(ctx); tid != "" {
			return context.WithValue(ctx, ctxTraceIDKey, tid)
		}
	}
	return context.WithValue(ctx, ctxTraceIDKey, newUUID())
}

// newUUID sinh UUID v4 chuẩn RFC 4122 bằng crypto/rand, không cần thư viện ngoài.
func newUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err) // hoặc xử lý lỗi phù hợp
	}
	// Set version (4) và variant bits theo RFC 4122
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return hex.EncodeToString(b[0:4]) + "-" +
		hex.EncodeToString(b[4:6]) + "-" +
		hex.EncodeToString(b[6:8]) + "-" +
		hex.EncodeToString(b[8:10]) + "-" +
		hex.EncodeToString(b[10:16])
}

// GetLogger trả về LoggerWithCtx từ context.
// Nếu context chưa có Logger, sẽ dùng globalLogger mặc định.
func GetLogger(ctx context.Context) LoggerWithCtx {
	ensureInit()
	var base *Logger
	if l, ok := ctx.Value(ctxLoggerKey{}).(*Logger); ok && l != nil {
		base = l
	} else {
		base = globalLogger
	}
	module, _ := ctx.Value(ctxModuleKey).(string)
	if module == "" {
		module = "unknown"
	}
	ctx = context.WithValue(ctx, ctxModuleKey, module)
	return LoggerWithCtx{l: base, ctx: ctx}
}

// Context trả về context từ LoggerWithCtx.
func (lw LoggerWithCtx) Context() context.Context {
	return lw.ctx
}

// ===== Các phương thức ghi log trên LoggerWithCtx =====

func (lw LoggerWithCtx) Debug(format string, args ...interface{}) {
	lw.l.log(lw.ctx, DEBUG, format, args...)
}

func (lw LoggerWithCtx) Info(format string, args ...interface{}) {
	lw.l.log(lw.ctx, INFO, format, args...)
}

func (lw LoggerWithCtx) Warn(format string, args ...interface{}) {
	lw.l.log(lw.ctx, WARN, format, args...)
}

func (lw LoggerWithCtx) Error(format string, args ...interface{}) {
	lw.l.log(lw.ctx, ERROR, format, args...)
}

func (lw LoggerWithCtx) Fatal(format string, args ...interface{}) {
	lw.l.log(lw.ctx, FATAL, format, args...)
	_ = Close(2 * time.Second)
	os.Exit(1)
}

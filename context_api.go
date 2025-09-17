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

// WithLogger gắn *Logger vào context và trả về context mới.
func WithLogger(ctx context.Context, l *Logger) context.Context {
	return context.WithValue(ctx, ctxLoggerKey{}, l)
}

// LoggerFromContext lấy *Logger đã gắn trong ctx (nếu có).
func LoggerFromContext(ctx context.Context) (*Logger, bool) {
	l, ok := ctx.Value(ctxLoggerKey{}).(*Logger)
	return l, ok
}

// WithModule gắn module vào ctx và trả về LoggerWithCtx mới.
func WithModule(ctx context.Context, module string) LoggerWithCtx {
	ensureInit()
	ctx = context.WithValue(ctx, ctxModuleKey, module)
	return GetLogger(ctx)
}

// WithTraceID gắn/ghi đè trace_id vào ctx.
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, ctxTraceIDKey, traceID)
}

// WithFlowID gắn flow_id vào ctx.
func WithFlowID(ctx context.Context, flowID string) context.Context {
	return context.WithValue(ctx, ctxFlowIDKey, flowID)
}

// WithAttrs gắn thêm attributes vào ctx; trùng key sẽ ghi đè.
func WithAttrs(ctx context.Context, attrs Fields) context.Context {
	if attrs == nil {
		return ctx
	}
	existing, _ := ctx.Value(ctxFieldsKey).(Fields)
	newMap := make(Fields, len(existing)+len(attrs))
	for k, v := range existing {
		newMap[k] = v
	}
	for k, v := range attrs {
		newMap[k] = v
	}
	return context.WithValue(ctx, ctxFieldsKey, newMap)
}

// EnsureTraceIDCtx đảm bảo ctx có trace_id; ưu tiên lấy từ OpenTelemetry khi được bật.
func EnsureTraceIDCtx(ctx context.Context) context.Context {
	if id, ok := ctx.Value(ctxTraceIDKey).(string); ok && id != "" {
		return ctx
	}
	globalMu.RLock()
	l := globalLogger
	globalMu.RUnlock()
	if l != nil && l.enableOTEL.Load() {
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

// GetLogger lấy LoggerWithCtx từ ctx; fallback global logger nếu chưa gắn.
func GetLogger(ctx context.Context) LoggerWithCtx {
	ensureInit()
	var base *Logger
	if l, ok := ctx.Value(ctxLoggerKey{}).(*Logger); ok && l != nil {
		base = l
	} else {
		globalMu.RLock()
		base = globalLogger
		globalMu.RUnlock()
	}
	module, _ := ctx.Value(ctxModuleKey).(string)
	if module == "" {
		module = "unknown"
	}
	ctx = context.WithValue(ctx, ctxModuleKey, module)
	return LoggerWithCtx{l: base, ctx: ctx}
}

// Context trả về context bên trong LoggerWithCtx.
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

func (lw LoggerWithCtx) Fatal(format string, args []interface{}, fields Fields) {
	lw.l.log(lw.ctx, FATAL, format, args, fields)
	_ = CloseDetached(lw.l, 2*time.Second)
	os.Exit(1)
}

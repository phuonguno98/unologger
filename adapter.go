// Copyright (c) 2025 Nguyễn Thanh Phương
// This source code is licensed under the MIT License found in the LICENSE file.

// Package unologger - adapter.go
// Cung cấp Adapter bọc LoggerWithCtx để ghi log không cần truyền context ở mỗi lần gọi.
// Hỗ trợ cả interface SimpleLogger tối giản và ExtendedLogger có thêm Fatal.

package unologger

import "context"

// SimpleLogger là interface ghi log tối giản cho gói bên ngoài.
type SimpleLogger interface {
	// Debug ghi log cấp DEBUG.
	Debug(format string, args ...interface{})
	// Info ghi log cấp INFO.
	Info(format string, args ...interface{})
	// Warn ghi log cấp WARN.
	Warn(format string, args ...interface{})
	// Error ghi log cấp ERROR.
	Error(format string, args ...interface{})
}

// ExtendedLogger mở rộng SimpleLogger với phương thức Fatal.
type ExtendedLogger interface {
	SimpleLogger
	// Fatal ghi log cấp FATAL.
	Fatal(format string, args ...interface{})
}

// Adapter bọc LoggerWithCtx để giữ metadata và gọi log ngắn gọn.
type Adapter struct {
	lw LoggerWithCtx
}

// NewAdapter tạo Adapter từ LoggerWithCtx, panic nếu Logger nil.
func NewAdapter(lw LoggerWithCtx) *Adapter {
	if lw.l == nil {
		panic("unologger: NewAdapter received LoggerWithCtx with nil *Logger")
	}
	return &Adapter{lw: lw}
}

// NewAdapterFromContext tạo Adapter từ context; fallback global logger nếu chưa gắn.
func NewAdapterFromContext(ctx context.Context) *Adapter {
	return &Adapter{lw: GetLogger(ctx)}
}

// Context trả về context hiện tại của Adapter.
func (a *Adapter) Context() context.Context {
	return a.lw.Context()
}

// WithContext trả về Adapter mới với context thay đổi, dùng chung *Logger.
func (a *Adapter) WithContext(ctx context.Context) *Adapter {
	return &Adapter{lw: LoggerWithCtx{l: a.lw.l, ctx: ctx}}
}

// WithModule trả về Adapter mới gắn module vào context.
func (a *Adapter) WithModule(module string) *Adapter {
	lw := WithModule(a.lw.ctx, module)
	return &Adapter{lw: lw}
}

// WithTraceID trả về Adapter mới gắn/ghi đè trace_id vào context.
func (a *Adapter) WithTraceID(traceID string) *Adapter {
	return a.WithContext(WithTraceID(a.lw.ctx, traceID))
}

// WithFlowID trả về Adapter mới gắn flow_id vào context.
func (a *Adapter) WithFlowID(flowID string) *Adapter {
	return a.WithContext(WithFlowID(a.lw.ctx, flowID))
}

// WithAttrs trả về Adapter mới gắn thêm attributes vào context.
// Nếu trùng key, giá trị mới sẽ ghi đè.
func (a *Adapter) WithAttrs(attrs map[string]string) *Adapter {
	return a.WithContext(WithAttrs(a.lw.ctx, attrs))
}

// Debug ghi log cấp DEBUG qua Adapter.
func (a *Adapter) Debug(format string, args ...interface{}) {
	a.lw.Debug(format, args...)
}

// Info ghi log cấp INFO qua Adapter.
func (a *Adapter) Info(format string, args ...interface{}) {
	a.lw.Info(format, args...)
}

// Warn ghi log cấp WARN qua Adapter.
func (a *Adapter) Warn(format string, args ...interface{}) {
	a.lw.Warn(format, args...)
}

// Error ghi log cấp ERROR qua Adapter.
func (a *Adapter) Error(format string, args ...interface{}) {
	a.lw.Error(format, args...)
}

// Fatal ghi log cấp FATAL và kết thúc tiến trình theo chuẩn LoggerWithCtx.Fatal.
func (a *Adapter) Fatal(format string, args ...interface{}) {
	a.lw.Fatal(format, args...)
}

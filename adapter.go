// Copyright (c) 2025 Nguyễn Thanh Phương
// This source code is licensed under the MIT License found in the LICENSE file.

// Package unologger - adapter.go
// Cung cấp Adapter bọc LoggerWithCtx để ghi log không cần truyền context ở mỗi lần gọi.
// Hỗ trợ cả interface SimpleLogger tối giản và ExtendedLogger có thêm Fatal.

package unologger

import "context"

// SimpleLogger là interface ghi log tối giản, thường dùng để truyền vào các package
// bên ngoài mà không cần biết chi tiết triển khai logger nội bộ.
//
// Các phương thức tương ứng với các cấp độ log phổ biến.
type SimpleLogger interface {
	Debug(format string, args ...interface{})
	Info(format string, args ...interface{})
	Warn(format string, args ...interface{})
	Error(format string, args ...interface{})
}

// ExtendedLogger kế thừa SimpleLogger và bổ sung cấp độ FATAL.
// Hữu ích khi bạn muốn cho phép package bên ngoài có thể ghi log FATAL.
type ExtendedLogger interface {
	SimpleLogger
	Fatal(format string, args ...interface{})
}

// Adapter bọc một LoggerWithCtx, giúp gọi các phương thức log ngắn gọn
// mà vẫn giữ đầy đủ metadata (module, trace_id, flow_id, attrs) trong context.
// Adapter triển khai cả SimpleLogger và ExtendedLogger.
type Adapter struct {
	lw LoggerWithCtx
}

// NewAdapter tạo Adapter từ một LoggerWithCtx.
// Panics nếu logger bên trong là nil.
func NewAdapter(lw LoggerWithCtx) *Adapter {
	if lw.l == nil {
		panic("unologger: NewAdapter received LoggerWithCtx with nil *Logger")
	}
	return &Adapter{lw: lw}
}

// NewAdapterFromContext tạo Adapter bằng cách lấy LoggerWithCtx từ context.
// Nếu ctx chưa gắn logger, sẽ dùng logger mặc định đã được khởi tạo trước đó.
func NewAdapterFromContext(ctx context.Context) *Adapter {
	return &Adapter{lw: GetLogger(ctx)}
}

// Context trả về context hiện tại mà Adapter đang nắm giữ.
func (a *Adapter) Context() context.Context {
	return a.lw.Context()
}

// WithContext trả về một Adapter mới có cùng *Logger nhưng thay context.
func (a *Adapter) WithContext(ctx context.Context) *Adapter {
	return &Adapter{lw: LoggerWithCtx{l: a.lw.l, ctx: ctx}}
}

// WithModule trả về Adapter mới, gắn module vào context hiện tại.
func (a *Adapter) WithModule(module string) *Adapter {
	lw := WithModule(a.lw.ctx, module)
	return &Adapter{lw: lw}
}

// WithTraceID trả về Adapter mới, gắn hoặc ghi đè trace_id trong context.
func (a *Adapter) WithTraceID(traceID string) *Adapter {
	return a.WithContext(WithTraceID(a.lw.ctx, traceID))
}

// WithFlowID trả về Adapter mới, gắn flow_id vào context.
func (a *Adapter) WithFlowID(flowID string) *Adapter {
	return a.WithContext(WithFlowID(a.lw.ctx, flowID))
}

// WithAttrs trả về Adapter mới, gắn thêm các thuộc tính vào context.
// Nếu trùng key, giá trị mới sẽ ghi đè.
func (a *Adapter) WithAttrs(attrs map[string]string) *Adapter {
	return a.WithContext(WithAttrs(a.lw.ctx, attrs))
}

// Debug ghi log cấp DEBUG thông qua LoggerWithCtx bên trong.
func (a *Adapter) Debug(format string, args ...interface{}) {
	a.lw.Debug(format, args...)
}

// Info ghi log cấp INFO thông qua LoggerWithCtx bên trong.
func (a *Adapter) Info(format string, args ...interface{}) {
	a.lw.Info(format, args...)
}

// Warn ghi log cấp WARN thông qua LoggerWithCtx bên trong.
func (a *Adapter) Warn(format string, args ...interface{}) {
	a.lw.Warn(format, args...)
}

// Error ghi log cấp ERROR thông qua LoggerWithCtx bên trong.
func (a *Adapter) Error(format string, args ...interface{}) {
	a.lw.Error(format, args...)
}

// Fatal ghi log cấp FATAL thông qua LoggerWithCtx bên trong.
// Theo chuẩn gốc: ghi log, cố gắng Close với timeout, sau đó thoát tiến trình.
func (a *Adapter) Fatal(format string, args ...interface{}) {
	a.lw.Fatal(format, args...)
}

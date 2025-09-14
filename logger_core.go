// Copyright (c) 2025 Nguyễn Thanh Phương
// This source code is licensed under the MIT License found in the LICENSE file.

// Package unologger - logger_core.go
// Triển khai các phương thức ghi log trên Logger và LoggerWithCtx.
// Đây là phần lõi: nhận log entry, kiểm tra cấp độ, đưa vào pipeline xử lý.

package unologger

import (
	"context"
	"os"
	"time"
)

// Debug ghi log cấp DEBUG với context cho trước.
func (l *Logger) Debug(ctx context.Context, format string, args ...interface{}) {
	l.log(ctx, DEBUG, format, args...)
}

// Info ghi log cấp INFO với context cho trước.
func (l *Logger) Info(ctx context.Context, format string, args ...interface{}) {
	l.log(ctx, INFO, format, args...)
}

// Warn ghi log cấp WARN với context cho trước.
func (l *Logger) Warn(ctx context.Context, format string, args ...interface{}) {
	l.log(ctx, WARN, format, args...)
}

// Error ghi log cấp ERROR với context cho trước.
func (l *Logger) Error(ctx context.Context, format string, args ...interface{}) {
	l.log(ctx, ERROR, format, args...)
}

// Fatal ghi log cấp FATAL, cố gắng Close trong 2s rồi thoát tiến trình.
func (l *Logger) Fatal(ctx context.Context, format string, args ...interface{}) {
	l.log(ctx, FATAL, format, args...)
	_ = CloseDetached(l, 2*time.Second)
	os.Exit(1)
}

// WithContext trả về LoggerWithCtx mới gắn context cho Logger hiện tại.
func (l *Logger) WithContext(ctx context.Context) LoggerWithCtx {
	return LoggerWithCtx{l: l, ctx: ctx}
}

// GlobalLogger trả về global logger (tự init mặc định nếu chưa khởi tạo).
func GlobalLogger() *Logger {
	ensureInit()
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalLogger
}

// log là hàm nội bộ để ghi log với đầy đủ metadata và đưa vào pipeline.
// Được gọi bởi tất cả các phương thức ghi log.
func (l *Logger) log(ctx context.Context, level Level, format string, args ...interface{}) {
	// Kiểm tra cấp độ log tối thiểu
	if level < Level(l.minLevel.Load()) {
		return
	}

	// Tạo log entry từ pool
	entry := poolEntry.Get().(*logEntry)
	entry.lvl = level
	entry.ctx = ctx
	entry.t = time.Now()
	entry.tmpl = format
	entry.args = args

	// Đưa vào pipeline xử lý
	l.enqueue(entry)
}

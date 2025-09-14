// Copyright (c) 2025 Nguyễn Thanh Phương
// This source code is licensed under the MIT License found in the LICENSE file.

// Package unologger - stats_close.go
// Cung cấp API thống kê và đóng logger (global hoặc detached).
// Hỗ trợ đầy đủ các cấp độ log: DEBUG, INFO, WARN, ERROR, FATAL.

package unologger

import (
	"fmt"
	"os"
	"time"
)

// Stats trả về thống kê của global logger:
//   - dropped: số log bị drop ở enqueue
//   - written: số log đã xử lý thành công
//   - batches: số batch đã flush
//   - writeErrs: tổng số lỗi ghi
//   - hookErrs: tổng số lỗi hook
//   - queueLen: số phần tử còn lại trong hàng đợi
//   - writerErrs: map tên writer -> số lỗi ghi
//   - hookErrLog: slice lỗi hook chi tiết (nếu có)
func Stats() (dropped, written, batches, writeErrs, hookErrs int64, queueLen int, writerErrs map[string]int64, hookErrLog []HookError) {
	if globalLogger == nil {
		return 0, 0, 0, 0, 0, 0, nil, nil
	}
	l := globalLogger
	return l.droppedCount.Load(),
		l.writtenCount.Load(),
		l.batchCount.Load(),
		l.writeErrCount.Load(),
		l.hookErrCount.Load(),
		len(l.ch),
		l.getWriterErrorStats(),
		l.getHookErrorLog()
}

// StatsDetached trả về thống kê của một logger riêng (detached logger).
func StatsDetached(l *Logger) (dropped, written, batches, writeErrs, hookErrs int64, queueLen int, writerErrs map[string]int64, hookErrLog []HookError) {
	if l == nil {
		return 0, 0, 0, 0, 0, 0, nil, nil
	}
	return l.droppedCount.Load(),
		l.writtenCount.Load(),
		l.batchCount.Load(),
		l.writeErrCount.Load(),
		l.hookErrCount.Load(),
		len(l.ch),
		l.getWriterErrorStats(),
		l.getHookErrorLog()
}

// Close đóng global logger: ngừng nhận log mới, flush batch, dừng worker, dừng hooks, đóng writers.
// timeout: thời gian tối đa chờ tất cả hoàn tất; <=0 nghĩa là chờ vô hạn.
// Idempotent: gọi nhiều lần không lỗi.
// Trả về lỗi nếu hết thời gian chờ mà chưa đóng xong.
func Close(timeout time.Duration) error {
	if globalLogger == nil || globalLogger.closed.IsTrue() {
		return nil
	}
	return closeLogger(globalLogger, timeout)
}

// CloseDetached đóng một logger riêng (detached logger).
func CloseDetached(l *Logger, timeout time.Duration) error {
	if l == nil || l.closed.IsTrue() {
		return nil
	}
	return closeLogger(l, timeout)
}

// closeLogger thực hiện đóng logger chung cho cả global và detached.
func closeLogger(l *Logger, timeout time.Duration) error {
	l.closed.SetTrue()
	close(l.ch) // báo worker pipeline dừng

	// Đóng hook runner nếu async
	l.closeHookRunner()

	// Đợi worker pipeline kết thúc
	done := make(chan struct{})
	go func() {
		l.wg.Wait()
		close(done)
	}()

	// Đóng writer phụ và rotationSink
	l.closeAllWriters()

	// In thống kê lỗi writer nếu có
	statsStr := l.formatWriterErrorStats()
	if statsStr != "no writer errors" {
		// Ghi ra stderr để dễ thấy
		_, _ = fmt.Fprintln(os.Stderr, statsStr)
	}

	if timeout <= 0 {
		<-done
		return nil
	}
	select {
	case <-done:
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("logger: close timeout after %s", timeout)
	}
}

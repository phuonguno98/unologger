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

// Stats trả về thống kê của global logger: dropped, written, batches, writeErrs,
// hookErrs, queueLen, writerErrs và hookErrLog.
func Stats() (dropped, written, batches, writeErrs, hookErrs int64, queueLen int, writerErrs map[string]int64, hookErrLog []HookError) {
	globalMu.RLock()
	l := globalLogger
	globalMu.RUnlock()
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

// StatsDetached trả về thống kê của một logger riêng (detached).
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

// Close đóng global logger: chặn log mới, chờ worker, dừng hooks, đóng writers.
// timeout: thời gian tối đa chờ tất cả hoàn tất; <=0 nghĩa là chờ vô hạn.
// Idempotent: gọi nhiều lần không lỗi.
// Trả về lỗi nếu hết thời gian chờ mà chưa đóng xong.
func Close(timeout time.Duration) error {
	globalMu.RLock()
	l := globalLogger
	globalMu.RUnlock()
	if l == nil || l.closed.IsTrue() {
		return nil
	}
	return closeLogger(l, timeout)
}

// CloseDetached đóng logger riêng với quy trình tương tự Close.
func CloseDetached(l *Logger, timeout time.Duration) error {
	if l == nil || l.closed.IsTrue() {
		return nil
	}
	return closeLogger(l, timeout)
}

// closeLogger thực hiện đóng logger chung cho cả global và detached.
func closeLogger(l *Logger, timeout time.Duration) error {
	// NEW: chỉ cho phép đóng một lần (tránh panic: close of closed channel)
	if !l.closed.TrySetTrue() {
		return nil
	}
	close(l.ch) // báo worker pipeline dừng

	done := make(chan struct{})
	go func() {
		l.wg.Wait()
		close(done)
	}()

	if timeout <= 0 {
		<-done
		// Worker đã dừng, giờ mới đóng hook runner và writers
		l.closeHookRunner()
		l.closeAllWriters()
		statsStr := l.formatWriterErrorStats()
		if statsStr != "no writer errors" {
			_, _ = fmt.Fprintln(os.Stderr, statsStr)
		}
		return nil
	}

	select {
	case <-done:
		l.closeHookRunner()
		l.closeAllWriters()
		statsStr := l.formatWriterErrorStats()
		if statsStr != "no writer errors" {
			_, _ = fmt.Fprintln(os.Stderr, statsStr)
		}
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("logger: close timeout after %s", timeout)
	}
}

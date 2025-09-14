// Copyright (c) 2025 Nguyễn Thanh Phương
// This source code is licensed under the MIT License found in the LICENSE file.

// Package unologger - writers.go
// Cung cấp các hàm ghi log ra nhiều đích (stdout, stderr, rotation file, writer phụ)
// với cơ chế retry/backoff và thống kê lỗi per-writer.
// Hỗ trợ đầy đủ các cấp độ log: DEBUG, INFO, WARN, ERROR, FATAL.

package unologger

import (
	"fmt"
	"io"
	"math/rand"
	"time"
)

// writeToAll ghi dữ liệu ra writer chính, rotationSink (nếu có) và tất cả writer phụ.
// Áp dụng retry/backoff theo cấu hình retryPolicy.
// Ghi nhận lỗi vào thống kê per-writer.
//
// Tham số:
//   - p: dữ liệu log đã format.
//   - isError: true nếu log thuộc cấp độ ERROR hoặc FATAL (ghi ra stderr), false nếu ngược lại.
func (l *Logger) writeToAll(p []byte, isError bool) {
	// Writer chính
	if isError {
		l.safeWrite("stderr", l.errOut, p)
	} else {
		l.safeWrite("stdout", l.stdOut, p)
	}

	// Writer rotation nội bộ (nếu có)
	if l.rotationSink != nil && l.rotationSink.Writer != nil {
		l.safeWrite(l.rotationSink.Name, l.rotationSink.Writer, p)
	}

	// Writer phụ
	for _, sink := range l.extraW {
		l.safeWrite(sink.Name, sink.Writer, p)
	}
}

// safeWrite ghi ra một writer với retry/backoff và thống kê lỗi.
// Nếu ghi thành công, thoát ngay. Nếu lỗi, thử lại theo retryPolicy.
func (l *Logger) safeWrite(name string, w io.Writer, p []byte) {
	if w == nil {
		return // Không ghi nếu writer nil
	}

	var err error
	delay := l.retryPolicy.Backoff
	for attempt := 0; attempt <= l.retryPolicy.MaxRetries; attempt++ {
		_, err = w.Write(p)
		if err == nil {
			return
		}
		// Ghi nhận lỗi
		l.writeErrCount.Add(1)
		l.incWriterErr(name)

		// Nếu không retry hoặc đã hết số lần retry
		if attempt == l.retryPolicy.MaxRetries {
			return
		}

		// Tính delay
		sleep := delay
		if l.retryPolicy.Exponential {
			sleep = delay * (1 << attempt)
		}
		if l.retryPolicy.Jitter > 0 {
			j := time.Duration(rand.Int63n(int64(l.retryPolicy.Jitter)))
			sleep += j
		}
		time.Sleep(sleep)
	}
}

// incWriterErr tăng bộ đếm lỗi cho writer cụ thể.
func (l *Logger) incWriterErr(name string) {
	val, _ := l.writerErrs.LoadOrStore(name, &atomicI64{})
	val.(*atomicI64).Add(1)
}

// closeAllWriters đóng rotationSink và tất cả writer phụ nếu chúng implement io.Closer.
// Được gọi khi Close() hoặc CloseDetached().
func (l *Logger) closeAllWriters() {
	// rotationSink
	if l.rotationSink != nil && l.rotationSink.Closer != nil {
		if err := l.rotationSink.Closer.Close(); err != nil {
			l.writeErrCount.Add(1)
			l.incWriterErr(l.rotationSink.Name)
		}
	}
	// extras
	for _, sink := range l.extraW {
		if sink.Closer != nil {
			if err := sink.Closer.Close(); err != nil {
				// Ghi nhận lỗi đóng writer
				l.writeErrCount.Add(1)
				l.incWriterErr(sink.Name)
			}
		}
	}
}

// getWriterErrorStats trả về map tên writer -> số lỗi ghi.
func (l *Logger) getWriterErrorStats() map[string]int64 {
	stats := make(map[string]int64)
	l.writerErrs.Range(func(key, value interface{}) bool {
		if name, ok := key.(string); ok {
			if cnt, ok2 := value.(*atomicI64); ok2 {
				stats[name] = cnt.Load()
			}
		}
		return true
	})
	return stats
}

// formatWriterErrorStats trả về chuỗi thống kê lỗi writer.
// Ví dụ: "writer errors: stderr=2 stdout=0 rotation=1"
func (l *Logger) formatWriterErrorStats() string {
	stats := l.getWriterErrorStats()
	if len(stats) == 0 {
		return "no writer errors"
	}
	s := "writer errors:"
	for name, cnt := range stats {
		s += fmt.Sprintf(" %s=%d", name, cnt)
	}
	return s
}

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
	"sort"
	"time"
)

// writeToAll ghi buffer ra stdout/stderr (tùy isError), rotation và toàn bộ extra writers.
func (l *Logger) writeToAll(p []byte, isError bool) {
	// Snapshot outputs để không giữ khóa trong lúc I/O
	l.outputsMu.RLock()
	std := l.stdOut
	errw := l.errOut
	var rotName string
	var rotWriter io.Writer
	if l.rotationSink != nil && l.rotationSink.Writer != nil {
		rotName = l.rotationSink.Name
		rotWriter = l.rotationSink.Writer
	}
	extras := make([]writerSink, len(l.extraW))
	copy(extras, l.extraW)
	l.outputsMu.RUnlock()

	// Writer chính
	if isError {
		l.safeWrite("stderr", errw, p)
	} else {
		l.safeWrite("stdout", std, p)
	}

	// Writer rotation nội bộ (nếu có)
	if rotWriter != nil {
		l.safeWrite(rotName, rotWriter, p)
	}

	// Writer phụ
	for _, sink := range extras {
		l.safeWrite(sink.Name, sink.Writer, p)
	}
}

// safeWrite ghi ra một writer với retry/backoff và thống kê lỗi.
// Nếu ghi thành công, thoát ngay. Nếu lỗi, thử lại theo retryPolicy.
func (l *Logger) safeWrite(name string, w io.Writer, p []byte) {
	if w == nil {
		return
	}

	// Snapshot retryPolicy an toàn
	l.dynConfig.mu.RLock()
	rp := l.retryPolicy
	l.dynConfig.mu.RUnlock()

	// Clamp cấu hình sai để luôn có ít nhất 1 lần ghi thử
	maxRetries := rp.MaxRetries
	if maxRetries < 0 {
		maxRetries = 0
	}
	delay := rp.Backoff
	if delay < 0 {
		delay = 0
	}

	var err error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		_, err = w.Write(p)
		if err == nil {
			return
		}
		l.writeErrCount.Add(1)
		l.incWriterErr(name)

		if attempt == maxRetries {
			return
		}

		sleep := delay
		if rp.Exponential {
			sleep = delay * (1 << attempt)
		}
		if rp.Jitter > 0 {
			n := time.Now().UnixNano()
			if n < 0 {
				n = -n
			}
			j := time.Duration(n % int64(rp.Jitter))
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

// closeAllWriters đóng rotation và extra writers nếu có io.Closer.
func (l *Logger) closeAllWriters() {
	l.outputsMu.Lock()
	defer l.outputsMu.Unlock()

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

// formatWriterErrorStats trả về chuỗi tóm tắt số lỗi theo writer, sắp xếp theo tên.
func (l *Logger) formatWriterErrorStats() string {
	stats := l.getWriterErrorStats()
	if len(stats) == 0 {
		return "no writer errors"
	}
	// Sắp xếp theo tên để ổn định
	names := make([]string, 0, len(stats))
	for name := range stats {
		names = append(names, name)
	}
	sort.Strings(names)

	s := "writer errors:"
	for _, name := range names {
		s += fmt.Sprintf(" %s=%d", name, stats[name])
	}
	return s
}

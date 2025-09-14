// Copyright (c) 2025 Nguyễn Thanh Phương
// This source code is licensed under the MIT License found in the LICENSE file.

// Package unologger - hooks.go
// Quản lý hệ thống hook: cho phép chèn các hành vi tùy chỉnh trước hoặc sau khi ghi log.
// Hook có thể chạy đồng bộ hoặc bất đồng bộ, hỗ trợ timeout và chống panic.
// Áp dụng cho mọi cấp độ log: DEBUG, INFO, WARN, ERROR, FATAL.

package unologger

import (
	"context"
	"fmt"
	"time"
)

// startHookRunner khởi động worker pool xử lý hook khi ở chế độ async.
func (l *Logger) startHookRunner() {
	l.hooksMu.RLock()
	hasHooks := len(l.hooks) > 0
	l.hooksMu.RUnlock()
	if !l.hookAsync || !hasHooks {
		return
	}
	l.hookQueueCh = make(chan hookTask, l.hookQueue)
	for i := 0; i < l.hookWorkers; i++ {
		l.hookWg.Add(1)
		go func() {
			defer l.hookWg.Done()
			for task := range l.hookQueueCh {
				l.runHooks(task.event)
			}
		}()
	}
}

// enqueueHook đưa sự kiện vào hàng đợi hook async hoặc chạy sync nếu không async.
func (l *Logger) enqueueHook(ev HookEvent) {
	l.hooksMu.RLock()
	hasHooks := len(l.hooks) > 0
	l.hooksMu.RUnlock()
	if !hasHooks {
		return
	}
	if l.hookAsync {
		select {
		case l.hookQueueCh <- hookTask{event: ev}:
		default:
			l.recordHookError(ev, ErrHookQueueFull)
		}
	} else {
		l.runHooks(ev)
	}
}

// snapshotHooks trả về bản sao slice hooks hiện tại (không giữ khóa khi thực thi).
func (l *Logger) snapshotHooks() []HookFunc {
	l.hooksMu.RLock()
	defer l.hooksMu.RUnlock()
	if len(l.hooks) == 0 {
		return nil
	}
	cp := make([]HookFunc, len(l.hooks))
	copy(cp, l.hooks)
	return cp
}

// runHooks thực thi tất cả hooks với timeout và panic-safe.
func (l *Logger) runHooks(ev HookEvent) {
	hooks := l.snapshotHooks()
	if len(hooks) == 0 {
		return
	}
	for _, hk := range hooks {
		func() {
			defer func() {
				if r := recover(); r != nil {
					l.recordHookError(ev, ErrHookPanic)
				}
			}()
			if l.hookTimeout > 0 {
				ctx, cancel := context.WithTimeout(context.Background(), l.hookTimeout)
				defer cancel()
				done := make(chan struct{})
				var err error
				go func() {
					err = hk(ev)
					close(done)
				}()
				select {
				case <-ctx.Done():
					l.recordHookError(ev, ErrHookTimeout)
				case <-done:
					if err != nil {
						l.recordHookError(ev, err)
					}
				}
			} else {
				if err := hk(ev); err != nil {
					l.recordHookError(ev, err)
				}
			}
		}()
	}
}

// recordHookError ghi nhận lỗi hook vào bộ đếm và log chi tiết (có giới hạn).
func (l *Logger) recordHookError(ev HookEvent, err error) {
	l.hookErrCount.Add(1)
	l.hookErrMu.Lock()
	defer l.hookErrMu.Unlock()
	// Giới hạn số lỗi lưu lại để tránh tăng bộ nhớ vô hạn
	if l.hookErrMax <= 0 {
		l.hookErrMax = defaultHookErrMax
	}
	if len(l.hookErrLog) >= l.hookErrMax {
		// loại bỏ phần đầu để giữ lại (hookErrMax-1) phần tử mới nhất
		trim := len(l.hookErrLog) - (l.hookErrMax - 1)
		if trim < 1 {
			trim = 1
		}
		l.hookErrLog = append(l.hookErrLog[trim:], HookError{
			Time:    time.Now(),
			Level:   ev.Level,
			Module:  ev.Module,
			Message: ev.Message,
			Err:     err,
		})
		return
	}
	l.hookErrLog = append(l.hookErrLog, HookError{
		Time:    time.Now(),
		Level:   ev.Level,
		Module:  ev.Module,
		Message: ev.Message,
		Err:     err,
	})
}

// getHookErrorLog trả về bản sao các lỗi hook đã ghi nhận.
func (l *Logger) getHookErrorLog() []HookError {
	l.hookErrMu.Lock()
	defer l.hookErrMu.Unlock()
	out := make([]HookError, len(l.hookErrLog))
	copy(out, l.hookErrLog)
	return out
}

// closeHookRunner đóng hàng đợi hook và chờ worker kết thúc; có thể start lại sau đó.
func (l *Logger) closeHookRunner() {
	if l.hookAsync && l.hookQueueCh != nil {
		close(l.hookQueueCh)
		l.hookWg.Wait()
		// NEW: cho phép khởi động lại sau Close hoặc thay đổi hooks
		l.hookQueueCh = nil
	}
}

// ErrHookQueueFull báo hàng đợi hook đầy và sự kiện bị bỏ lỡ.
var ErrHookQueueFull = fmt.Errorf("hook queue full")

// ErrHookTimeout báo hook vượt quá timeout.
var ErrHookTimeout = fmt.Errorf("hook timeout")

// ErrHookPanic báo hook panic khi thực thi.
var ErrHookPanic = fmt.Errorf("hook panic")

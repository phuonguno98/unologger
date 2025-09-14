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

// startHookRunner khởi động worker pool cho hooks nếu cấu hình async.
func (l *Logger) startHookRunner() {
	if !l.hookAsync || len(l.hooks) == 0 {
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

// enqueueHook đẩy sự kiện hook vào hàng đợi async hoặc chạy sync nếu không async.
func (l *Logger) enqueueHook(ev HookEvent) {
	if len(l.hooks) == 0 {
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

// runHooks chạy tất cả hooks với timeout và panic-safe.
func (l *Logger) runHooks(ev HookEvent) {
	for _, hk := range l.hooks {
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

// recordHookError ghi nhận lỗi hook vào thống kê và lưu chi tiết.
func (l *Logger) recordHookError(ev HookEvent, err error) {
	l.hookErrCount.Add(1)
	l.hookErrMu.Lock()
	defer l.hookErrMu.Unlock()
	l.hookErrLog = append(l.hookErrLog, HookError{
		Time:    time.Now(),
		Level:   ev.Level,
		Module:  ev.Module,
		Message: ev.Message,
		Err:     err,
	})
}

// getHookErrorLog trả về bản sao slice lỗi hook chi tiết.
func (l *Logger) getHookErrorLog() []HookError {
	l.hookErrMu.Lock()
	defer l.hookErrMu.Unlock()
	out := make([]HookError, len(l.hookErrLog))
	copy(out, l.hookErrLog)
	return out
}

// closeHookRunner đóng hàng đợi hook và chờ worker kết thúc.
func (l *Logger) closeHookRunner() {
	if l.hookAsync && l.hookQueueCh != nil {
		close(l.hookQueueCh)
		l.hookWg.Wait()
	}
}

// Các lỗi hook chuẩn.
var (
	ErrHookQueueFull = fmt.Errorf("hook queue full")
	ErrHookTimeout   = fmt.Errorf("hook timeout")
	ErrHookPanic     = fmt.Errorf("hook panic")
)

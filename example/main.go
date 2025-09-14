// Copyright (c) 2025 Nguyễn Thanh Phương
// This source code is licensed under the MIT License found in the LICENSE file.

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/phuonguno98/unologger"
)

// Hàm bên ngoài 1: chỉ cần SimpleLogger
func doSomethingBasic(log unologger.SimpleLogger) {
	log.Info("Gọi từ package bên ngoài (SimpleLogger)")
	log.Warn("Cảnh báo từ package bên ngoài")
}

// Hàm bên ngoài 2: cần ExtendedLogger (có Fatal)
func doSomethingCritical(log unologger.ExtendedLogger) {
	log.Error("Lỗi nghiêm trọng từ package bên ngoài")
	// log.Fatal("Fatal từ package bên ngoài - sẽ dừng chương trình")
}

// Hàm bên ngoài 3: nhận logger qua context
func processPayment(ctx context.Context, orderID int) {
	log := unologger.GetLogger(ctx)
	log.Info("Bắt đầu xử lý thanh toán cho đơn hàng %d", orderID)
	ctx = unologger.WithAttrs(ctx, map[string]string{"order_id": fmt.Sprint(orderID)})
	log = unologger.GetLogger(ctx)
	log.Debug("Đã gắn thêm order_id vào context")
}

func sendEmail(ctx context.Context, to string) {
	ctx = unologger.WithModule(ctx, "email-service").Context()
	log := unologger.GetLogger(ctx)
	log.Info("Gửi email tới %s", to)
}

func main() {
	// 1. Cấu hình logger đầy đủ
	cfg := unologger.Config{
		MinLevel: unologger.DEBUG,
		Timezone: "Asia/Ho_Chi_Minh",
		JSON:     false,
		Buffer:   1024,
		Workers:  2,
		Batch:    unologger.BatchConfig{Size: 5, MaxWait: 500 * time.Millisecond},
		Retry:    unologger.RetryPolicy{MaxRetries: 2, Backoff: 100 * time.Millisecond, Exponential: true},
		Rotation: unologger.RotationConfig{
			Enable:     true,
			Filename:   "app.log",
			MaxSizeMB:  10,
			MaxBackups: 3,
			MaxAge:     7,
			Compress:   true,
		},
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		RegexPatternMap: map[string]string{
			`\b\d{16}\b`: "****MASKED_CARD****",
		},
		Hooks: []unologger.HookFunc{
			func(ev unologger.HookEvent) error {
				fmt.Printf("[HOOK] %s %s: %s\n", ev.Level, ev.Module, ev.Message)
				return nil
			},
		},
		EnableOTEL: false,
	}

	// 2. Khởi tạo logger toàn cục
	unologger.InitLoggerWithConfig(cfg)

	// 3. Tạo context với metadata
	ctx := context.Background()
	ctx = unologger.WithModule(ctx, "main-service").Context()
	ctx = unologger.WithTraceID(ctx, "trace-xyz")
	ctx = unologger.WithFlowID(ctx, "flow-001")
	ctx = unologger.WithAttrs(ctx, map[string]string{"user_id": "u001"})

	// 4. Lấy logger từ context
	log := unologger.GetLogger(ctx)

	// 5. Ghi log các cấp độ
	log.Debug("Bắt đầu xử lý thanh toán cho user %s", "u001")
	log.Info("Thanh toán thành công cho đơn hàng %d", 1001)
	log.Warn("Số dư tài khoản thấp cho user %s", "u001")
	log.Error("Lỗi kết nối tới ngân hàng")

	// 6. Log có dữ liệu nhạy cảm sẽ bị mask
	log.Info("Số thẻ: 1234567812345678")

	// 7. Dùng Adapter để truyền logger vào package bên ngoài
	adapter := unologger.NewAdapter(log)
	doSomethingBasic(adapter)    // SimpleLogger
	doSomethingCritical(adapter) // ExtendedLogger

	// 8. Gọi hàm bên ngoài nhận logger qua context
	processPayment(ctx, 1002)
	sendEmail(ctx, "user@example.com")

	// 9. Thay đổi cấu hình động khi runtime
	log.Info("Thay đổi cấp độ log tối thiểu thành WARN")
	unologger.GlobalLogger().SetMinLevel(unologger.WARN)
	log.Debug("Log này sẽ bị bỏ qua vì cấp độ < WARN")
	log.Warn("Log này sẽ được ghi vì >= WARN")

	// 10. Đóng logger và in thống kê lỗi writer
	if err := unologger.Close(2 * time.Second); err != nil {
		fmt.Println("Đóng logger bị timeout:", err)
	}
}

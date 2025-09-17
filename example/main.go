// Copyright (c) 2025 Nguyễn Thanh Phương
// This source code is licensed under the MIT License found in the LICENSE file.

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"go.opentelemetry.io/otel"

	"github.com/phuonguno98/unologger"
)

// hookPrint in sự kiện hook ra stdout
func hookPrint(ev unologger.HookEvent) error {
	fmt.Printf("[HOOK] json=%v %s %s: %s\n", ev.JSONMode, ev.Level, ev.Module, ev.Message)
	return nil
}

// hookSlow giả lập xử lý chậm để minh họa timeout
func hookSlow(_ unologger.HookEvent) error {
	time.Sleep(300 * time.Millisecond)
	return nil
}

func main() {
	// 1) Cấu hình logger ban đầu (đầy đủ tính năng phổ biến)
	cfg := unologger.Config{
		MinLevel:    unologger.DEBUG,
		Timezone:    "Asia/Ho_Chi_Minh",
		JSON:        false,
		Buffer:      1024,
		Workers:     2,
		NonBlocking: true,
		DropOldest:  true,
		Batch:       unologger.BatchConfig{Size: 5, MaxWait: 400 * time.Millisecond},
		Retry:       unologger.RetryPolicy{MaxRetries: 2, Backoff: 80 * time.Millisecond, Exponential: true, Jitter: 20 * time.Millisecond},
		Rotation: unologger.RotationConfig{
			Enable:     true,
			Filename:   "example/app.log",
			MaxSizeMB:  5,
			MaxBackups: 2,
			MaxAge:     7,
			Compress:   true,
		},
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		RegexPatternMap: map[string]string{
			`\b\d{16}\b`:                                       "****MASKED_CARD****",
			`(?i)authorization:\s*Bearer\s+\S+`:                "authorization: Bearer ****MASKED****",
			`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`: "***@masked.email",
		},
		JSONFieldRules: []unologger.MaskFieldRule{
			{Keys: []string{"password", "token", "secret"}, Replacement: "****"},
		},
		Hooks:      []unologger.HookFunc{hookPrint, hookSlow},
		Hook:       unologger.HookConfig{Async: true, Workers: 2, Queue: 1024, Timeout: 200 * time.Millisecond},
		EnableOTEL: false, // sẽ bật sau để demo
	}
	unologger.InitLoggerWithConfig(cfg)
	defer func() {
		if err := unologger.Close(2 * time.Second); err != nil {
			fmt.Println("Close timeout:", err)
		}
	}()

	// 2) Ngữ cảnh ban đầu và LoggerWithCtx
	ctx := context.Background()
	ctx = unologger.WithModule(ctx, "main-service").Context()
	ctx = unologger.WithFlowID(ctx, "flow-001")
	ctx = unologger.WithAttrs(ctx, unologger.Fields{"user_id": "u001"})
	ctx = unologger.EnsureTraceIDCtx(ctx)
	log := unologger.GetLogger(ctx)

	// 3) Ghi log các cấp độ và masking text
	log.Debug("Bắt đầu xử lý thanh toán cho user %s", "u001")
	log.Info("Thanh toán thành công cho đơn hàng %d", 1001)
	log.Warn("Số dư tài khoản thấp cho user %s", "u001")
	log.Error("Lỗi kết nối tới ngân hàng")
	log.Info("Thử mask thẻ: 1234567812345678 và email: user@example.com")
	log.Info("Header authorization: Bearer very-secret-token-abcxyz")

	// 4) Adapter cho package bên ngoài
	adapter := unologger.NewAdapter(log)
	doExternal(adapter)

	// 5) Cấu hình động: min-level và batch
	unologger.GlobalLogger().SetMinLevel(unologger.WARN)
	log.Debug("Log này sẽ bị bỏ qua (DEBUG < WARN)")
	log.Warn("Log này sẽ được ghi (>= WARN)")
	unologger.GlobalLogger().SetBatchConfig(unologger.BatchConfig{Size: 3, MaxWait: 200 * time.Millisecond})

	// 6) Thêm writer phụ và đổi outputs
	tmpFile, _ := os.CreateTemp("", "extra-log-*.log")
	unologger.GlobalLogger().AddExtraWriter("tempfile", tmpFile)
	memBuf := &bytes.Buffer{}
	unologger.GlobalLogger().SetOutputs(nil, nil, []io.Writer{memBuf}, []string{"mem-buf"})
	log.Info("Ghi song song: stdout/stderr, rotation, tempfile và mem-buf")

	// 7) Cập nhật hooks khi runtime
	unologger.GlobalLogger().SetHooks([]unologger.HookFunc{
		func(ev unologger.HookEvent) error {
			fmt.Printf("[HOOK2] json=%v %s %s: %s\n", ev.JSONMode, ev.Level, ev.Module, ev.Message)
			return fmt.Errorf("demo hook error")
		},
	})

	// 8) Bật JSON mode và masking field-level JSON
	unologger.GlobalLogger().SetJSONFormat(true)
	log.Info(`{"event":"login","user":"u001","password":"123456","token":"abcdef"}`)

	// 9) Đổi timezone và bật OTEL
	if err := unologger.GlobalLogger().SetTimezone("UTC"); err != nil {
		fmt.Println("Đổi timezone lỗi:", err)
	}
	unologger.GlobalLogger().SetEnableOTEL(true)

	// 10) Gắn trace/span từ OTel và ghi log
	ctxOT, span := otel.Tracer("demo-tracer").Start(ctx, "demo-operation")
	unologger.GetLogger(ctxOT).Info("Log kèm trace/span từ OTel")
	span.End()

	// 11) Reinit toàn cục (đổi JSON, worker, min-level)
	cfg2 := cfg
	cfg2.JSON = true
	cfg2.Workers = 1
	cfg2.MinLevel = unologger.INFO
	if _, err := unologger.ReinitGlobalLogger(cfg2, 2*time.Second); err != nil {
		fmt.Println("Reinit logger lỗi:", err)
	}
	unologger.GetLogger(ctx).Info("Log sau ReinitGlobalLogger (JSON mode)")

	// 12) Loại bỏ writer phụ và đóng tệp tạm
	unologger.GlobalLogger().RemoveExtraWriter("tempfile")
	if tmpFile != nil {
		_ = tmpFile.Close()
	}

	// 13) In thống kê
	dropped, written, batches, werrs, herrs, qlen, wstats, hookErrLog := unologger.Stats()
	fmt.Printf("Stats: dropped=%d written=%d batches=%d writeErrs=%d hookErrs=%d queue=%d writers=%v hookErrLog=%d\n",
		dropped, written, batches, werrs, herrs, qlen, wstats, len(hookErrLog))

	// 14) Ghi thêm vài log để thấy batch flush
	for i := 0; i < 5; i++ {
		unologger.GetLogger(ctx).Info("Batch log %d", i+1)
	}
	time.Sleep(600 * time.Millisecond)

	// 15) (Tùy chọn) Gọi Fatal để minh họa hành vi thoát chương trình
	if os.Getenv("DEMO_FATAL") == "1" {
		unologger.GetLogger(ctx).Fatal("Demo FATAL: ứng dụng sẽ thoát", nil, unologger.Fields{"exit_code": 1})
	}
}

func doExternal(adapter *unologger.Adapter) {
	adapter.Info("Gọi từ package bên ngoài (SimpleLogger)")
	adapter.Warn("Cảnh báo từ package bên ngoài")
	adapter.Error("Lỗi từ package bên ngoài")
	// Tránh gọi Fatal trong ví dụ trừ khi được bật bằng biến môi trường
}

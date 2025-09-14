# unologger

**unologger** là thư viện ghi log bất đồng bộ cho Go, hỗ trợ nhiều cấp độ log, tích hợp context, masking dữ liệu nhạy cảm, xoay vòng file log, hook xử lý log, cấu hình động và tích hợp OpenTelemetry.

## ✨ Tính năng nổi bật

- **Context-first**: Logger được truyền qua `context.Context`, giữ nguyên metadata (module, trace_id, flow_id, attrs).
- **Nhiều cấp độ log**: `DEBUG`, `INFO`, `WARN`, `ERROR`, `FATAL`.
- **Batch processing**: Gom nhiều log entry thành batch để giảm I/O.
- **Hook**: Chèn hành vi tùy chỉnh trước/sau khi ghi log.
- **Masking**: Che dữ liệu nhạy cảm (số thẻ, email, token, mật khẩu) bằng regex hoặc field-level.
- **Rotation**: Xoay file log theo dung lượng hoặc thời gian với [lumberjack](https://github.com/natefinch/lumberjack).
- **Dynamic config**: Thay đổi cấu hình log khi runtime.
- **OTel integration**: Gắn trace_id/span_id tự động từ OpenTelemetry.
- **Adapter**: Cung cấp interface `SimpleLogger`/`ExtendedLogger` để truyền logger vào package bên ngoài.

---

## 📦 Cài đặt

```bash
go get github.com/phuonguno98/unologger
```

## 🚀 Sử dụng cơ bản

```go
package main

import (
    "context"
    "github.com/phuonguno98/unologger"
)

func main() {
    // Khởi tạo logger mặc định
    unologger.InitLogger(unologger.DEBUG, "Asia/Ho_Chi_Minh")

    // Tạo context với module và trace_id
    ctx := context.Background()
    lw := unologger.WithModule(ctx, "main-service")
    ctx = lw.Context()
    ctx = unologger.WithTraceID(ctx, "trace-12345")

    // Lấy logger từ context
    log := unologger.GetLogger(ctx)

    log.Info("Xin chào từ unologger!")
    log.Error("Có lỗi xảy ra")

    _ = unologger.Close(2 * time.Second)
}
```

## 💡 Ví dụ nâng cao (toàn diện)

```go
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

    unologger.InitLoggerWithConfig(cfg)

    ctx := context.Background()
    lw := unologger.WithModule(ctx, "main-service")
    ctx = lw.Context()
    ctx = unologger.WithTraceID(ctx, "trace-xyz")
    ctx = unologger.WithFlowID(ctx, "flow-001")
    ctx = unologger.WithAttrs(ctx, map[string]string{"user_id": "u001"})

    log := unologger.GetLogger(ctx)

    log.Debug("Bắt đầu xử lý thanh toán cho user %s", "u001")
    log.Info("Thanh toán thành công cho đơn hàng %d", 1001)
    log.Warn("Số dư tài khoản thấp cho user %s", "u001")
    log.Error("Lỗi kết nối tới ngân hàng")
    log.Info("Số thẻ: 1234567812345678") // sẽ bị mask

    adapter := unologger.NewAdapter(log)
    doSomethingBasic(adapter)
    doSomethingCritical(adapter)

    processPayment(ctx, 1002)
    sendEmail(ctx, "user@example.com")

    log.Info("Thay đổi cấp độ log tối thiểu thành WARN")
    unologger.GlobalLogger().SetMinLevel(unologger.WARN)
    log.Debug("Log này sẽ bị bỏ qua vì cấp độ < WARN")
    log.Warn("Log này sẽ được ghi vì >= WARN")

    if err := unologger.Close(2 * time.Second); err != nil {
        fmt.Println("Đóng logger bị timeout:", err)
    }
}
```

Output trong `app.log`:
```
2025-09-14 10:28:02.874 +07 [INFO] (main-service) trace=trace-xyz flow=flow-001 attrs=map[user_id:u001] Thanh toán thành công cho đơn hàng 1001
2025-09-14 10:28:02.874 +07 [DEBUG] (main-service) trace=trace-xyz flow=flow-001 attrs=map[user_id:u001] Bắt đầu xử lý thanh toán cho user u001
2025-09-14 10:28:02.874 +07 [WARN] (main-service) trace=trace-xyz flow=flow-001 attrs=map[user_id:u001] Cảnh báo từ package bên ngoài
2025-09-14 10:28:02.874 +07 [ERROR] (main-service) trace=trace-xyz flow=flow-001 attrs=map[user_id:u001] Lỗi nghiêm trọng từ package bên ngoài
2025-09-14 10:28:02.874 +07 [INFO] (main-service) trace=trace-xyz flow=flow-001 attrs=map[user_id:u001] Bắt đầu xử lý thanh toán cho đơn hàng 1002
2025-09-14 10:28:02.874 +07 [WARN] (main-service) trace=trace-xyz flow=flow-001 attrs=map[user_id:u001] Số dư tài khoản thấp cho user u001
2025-09-14 10:28:02.874 +07 [DEBUG] (main-service) trace=trace-xyz flow=flow-001 attrs=map[order_id:1002 user_id:u001] Đã gắn thêm order_id vào context
2025-09-14 10:28:02.874 +07 [INFO] (email-service) trace=trace-xyz flow=flow-001 attrs=map[user_id:u001] Gửi email tới user@example.com
2025-09-14 10:28:02.874 +07 [ERROR] (main-service) trace=trace-xyz flow=flow-001 attrs=map[user_id:u001] Lỗi kết nối tới ngân hàng
2025-09-14 10:28:02.874 +07 [INFO] (main-service) trace=trace-xyz flow=flow-001 attrs=map[user_id:u001] Thay đổi cấp độ log tối thiểu thành WARN
2025-09-14 10:28:02.874 +07 [WARN] (main-service) trace=trace-xyz flow=flow-001 attrs=map[user_id:u001] Log này sẽ được ghi vì >= WARN
2025-09-14 10:28:02.874 +07 [INFO] (main-service) trace=trace-xyz flow=flow-001 attrs=map[user_id:u001] Số thẻ: ****MASKED_CARD****
2025-09-14 10:28:02.874 +07 [INFO] (main-service) trace=trace-xyz flow=flow-001 attrs=map[user_id:u001] Gọi từ package bên ngoài (SimpleLogger)
```

## 📚 API chính

- **Khởi tạo**
  - `InitLogger(minLevel Level, timezone string)`
  - `InitLoggerWithConfig(cfg Config)`
  - `NewDetachedLogger(cfg Config) *Logger`
- **Context helpers**
  - `WithModule(ctx, module string) LoggerWithCtx`
  - `WithTraceID(ctx, traceID string) context.Context`
  - `WithFlowID(ctx, flowID string) context.Context`
  - `WithAttrs(ctx, attrs map[string]string) context.Context`
  - `GetLogger(ctx context.Context) LoggerWithCtx`
- **Adapter**
  - `NewAdapter(lw LoggerWithCtx) *Adapter`
  - `SimpleLogger` / `ExtendedLogger` interfaces
- **Dynamic config**
  - `SetMinLevel(level Level)`
  - `SetRegexRules(rules []MaskRuleRegex)`
  - `SetJSONFieldRules(rules []MaskFieldRule)`
- **Đóng logger**
  - `Close(timeout time.Duration) error`
  - `CloseDetached(l *Logger, timeout time.Duration) error`

## ⚡ Hiệu năng & An toàn

- Bất đồng bộ và batch giúp giảm overhead I/O.
- Masking regex có thể ảnh hưởng hiệu năng nếu pattern phức tạp — nên tối ưu pattern.
- `FATAL` sẽ gọi `os.Exit(1)` sau khi flush log — chỉ dùng khi thực sự cần dừng chương trình.

---

## 📄 License

MIT License — Xem file [LICENSE](LICENSE) để biết chi tiết.

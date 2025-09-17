# unologger

unologger là thư viện logging bất đồng bộ cho Go, tập trung vào hiệu năng, an toàn cạnh tranh và tính linh hoạt khi vận hành. Thư viện hỗ trợ batching, hooks, masking dữ liệu nhạy cảm, xoay file, cấu hình động và tích hợp OpenTelemetry.

## Tính năng chính

- Nhiều cấp độ log: `DEBUG`, `INFO`, `WARN`, `ERROR`, `FATAL`
- Batching bất đồng bộ, non-blocking queue với chính sách `DropOldest`
- Masking dữ liệu nhạy cảm bằng regex và theo tên field JSON
- Hooks sync/async với timeout và panic-safe, theo dõi lỗi hook
- Rotation file log bằng lumberjack, đa writer (stdout, stderr, extras)
- Cấu hình động: min-level, batch, retry, hooks, outputs, rotation, JSON mode, timezone
- Tích hợp OTel: tự động gắn trace/span ID từ context

## An toàn cạnh tranh và tối ưu hiệu năng

- Truy cập global logger an toàn với `RWMutex`
- JSON mode và OTEL flag dùng atomic, tránh race khi bật/tắt runtime
- Batch size và batch wait dùng atomic, worker cập nhật ngay lập tức
- Timer worker được reset theo cấu hình mới, không cần restart
- Outputs snapshot trước khi I/O, tránh giữ khóa khi ghi chậm
- Close idempotent với `TrySetTrue`, tránh “close of closed channel”
- `DropOldest` thu hồi entry bị drop về pool, tránh rò rỉ
- Hook runner có thể start lại sau khi Close (hookQueueCh reset)

## Yêu cầu

- Go ≥ 1.25, toolchain go1.25.1

## 📦 Cài đặt

```bash
go get github.com/phuonguno98/unologger
```

## 🚀 Quick Start

```go
package main

import (
	"context"
	"time"

	"github.com/phuonguno98/unologger"
)

func main() {
	unologger.InitLogger(unologger.INFO, "UTC")
	defer unologger.Close(2 * time.Second)

	ctx := unologger.WithModule(context.Background(), "app").Context()
	unologger.GetLogger(ctx).Info("hello %s", "world")
}
```

## 💡 Ví dụ đầy đủ (rút gọn)

```go
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/phuonguno98/unologger"
)

func main() {
	cfg := unologger.Config{
		MinLevel: unologger.DEBUG,
		Timezone: "Asia/Ho_Chi_Minh",
		JSON:     false,
		Buffer:   1024,
		Workers:  2,
		NonBlocking: true,
		DropOldest:  true,
		Batch:  unologger.BatchConfig{Size: 5, MaxWait: 400 * time.Millisecond},
		Retry:  unologger.RetryPolicy{MaxRetries: 2, Backoff: 80 * time.Millisecond, Exponential: true},
		Rotation: unologger.RotationConfig{
			Enable: true, Filename: "app.log", MaxSizeMB: 5, MaxBackups: 2, MaxAge: 7, Compress: true,
		},
		Stdout: os.Stdout, Stderr: os.Stderr,
		RegexPatternMap: map[string]string{`\b\d{16}\b`: "****MASKED_CARD****"},
		JSONFieldRules: []unologger.MaskFieldRule{{Keys: []string{"password", "token"}, Replacement: "****"}},
		Hooks: []unologger.HookFunc{
			func(ev unologger.HookEvent) error { fmt.Println("[HOOK]", ev.Level, ev.Message); return nil },
		},
	}

	unologger.InitLoggerWithConfig(cfg)
	defer unologger.Close(2 * time.Second)

	// Context + logging
	ctx := unologger.WithModule(context.Background(), "checkout").Context()
	ctx = unologger.WithFlowID(ctx, "flow-001")
	ctx = unologger.EnsureTraceIDCtx(ctx)
	log := unologger.GetLogger(ctx)

	log.Info("order %d created", 1001)
	log.Info(`{"event":"login","user":"u001","password":"123456"}`) // sẽ được mask khi JSON mode

	// Động cấu hình
	unologger.GlobalLogger().SetMinLevel(unologger.WARN)
	unologger.GlobalLogger().SetBatchConfig(unologger.BatchConfig{Size: 3, MaxWait: 200 * time.Millisecond})
	unologger.GlobalLogger().SetJSONFormat(true)
	_ = unologger.GlobalLogger().SetTimezone("UTC")

	// Tích hợp OTel (tùy chọn)
	// ctx, span := otel.Tracer("app").Start(ctx, "op")
	// unologger.GetLogger(ctx).Info("with OTel trace")
	// span.End()

	// Thống kê
	dropped, written, batches, werrs, herrs, qlen, wstats, hookErrLog := unologger.Stats()
	fmt.Println("stats:", dropped, written, batches, werrs, herrs, qlen, wstats, len(hookErrLog))
}
```

## Adapter cho package bên ngoài

```go
adapter := unologger.NewAdapter(unologger.GetLogger(ctx))
adapter.Info("message from external pkg")
adapter.Warn("warning from external pkg")
adapter.Error("error from external pkg")
// adapter.Fatal("fatal") // sẽ thoát tiến trình, chỉ bật khi thực sự cần
```

## Hooks

- `HookEvent` gồm: Time, Level, Module, Message, TraceID, FlowID, Attrs, JSONMode
- Cấu hình async: `HookConfig{Async, Workers, Queue, Timeout}`
- Theo dõi lỗi hook: `Stats` trả về count + danh sách lỗi gần đây

## Masking

- Regex: cung cấp `RegexRules` hoặc `RegexPatternMap`
- JSON field-level: `JSONFieldRules` theo tên trường; tiếp tục áp dụng regex sau khi mask JSON

## Rotation

- Cấu hình bằng lumberjack: `Filename`, `MaxSizeMB`, `MaxBackups`, `MaxAge`, `Compress`

## Outputs

- Thay đổi writer khi runtime: `SetOutputs(stdOut, errOut, extras, names)`
- Thêm/bớt writer phụ: `AddExtraWriter/RemoveExtraWriter`
- Writer errors có thể xem qua `Stats` và `formatWriterErrorStats` (in khi Close)

## Re-init toàn cục

- `ReinitGlobalLogger(cfg, timeout)` thay thế global logger an toàn
- Logger cũ sẽ được đóng theo timeout quy định

## Đóng logger

- `Close(timeout)` hoặc `CloseDetached(l, timeout)` an toàn, idempotent
- Close sẽ chặn log mới, chờ worker, dừng hooks, đóng writers và in thống kê lỗi writer

## Ghi chú về FATAL

- `Fatal` ghi log, cố gắng Close trong 2s rồi gọi `os.Exit(1)`
- Chỉ nên gọi ở cuối chương trình hoặc khi cần dừng khẩn cấp

## Kiểm thử an toàn luồng

- Chạy với race detector:

```bash
go build -race ./...
go run -race ./example
```

## 📄 License

MIT License — Xem file [LICENSE](LICENSE) để biết chi tiết.
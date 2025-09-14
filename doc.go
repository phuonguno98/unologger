// Copyright (c) 2025 Nguyễn Thanh Phương
// This source code is licensed under the MIT License found in the LICENSE file.

// Package unologger cung cấp logger bất đồng bộ cho Go với batching, hooks,
// masking dữ liệu nhạy cảm, xoay file, cấu hình động và tích hợp OpenTelemetry.
//
// Tính năng chính:
//   - Ghi log theo cấp độ (DEBUG, INFO, WARN, ERROR, FATAL).
//   - Gom batch để giảm I/O; hỗ trợ non-blocking với chính sách drop.
//   - Mask dữ liệu nhạy cảm bằng regex và theo tên trường JSON.
//   - Hooks đồng bộ/bất đồng bộ (timeout, panic-safe), theo dõi lỗi hook.
//   - Xoay file log bằng lumberjack, đa writer (stdout, stderr, extras).
//   - Cấu hình động: min-level, batch, retry, hooks, outputs, rotation, JSON mode, timezone.
//   - Tích hợp OTel: gắn trace/span ID từ context.
//
// Ví dụ:
//
//	cfg := unologger.Config{MinLevel: unologger.INFO, Timezone: "UTC", Buffer: 1024, Workers: 1}
//	unologger.InitLoggerWithConfig(cfg)
//	defer unologger.Close(2 * time.Second)
//	ctx := unologger.WithModule(context.Background(), "app").Context()
//	unologger.GetLogger(ctx).Info("hello %s", "world")
package unologger

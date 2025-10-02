# Prompt: Thêm slog Handler adapter cho unologger

Mục tiêu:
- Cung cấp một `slog.Handler` để người dùng dùng `log/slog` chuyển hướng log vào unologger.

Phạm vi:
- Tạo `slog_adapter.go` trong package `unologger`.
- Implement handler tối ưu hiệu năng, tận dụng pipeline/batching của unologger.
- Mapping level: slog DEBUG/INFO/WARN/ERROR → unologger DEBUG/INFO/WARN/ERROR.
- Convert `slog.Attr` → `Fields`, giữ kiểu dữ liệu serializable.
- Hỗ trợ context propagation (module/trace/flow/attrs) nếu có.
- Factory: `NewSlogHandler(lw LoggerWithCtx, opts ...Option) slog.Handler`.

Ràng buộc:
- GoDoc tiếng Anh, header bản quyền 2025, wrap 120.
- Không phá API public hiện có; không thêm deps nặng.

Acceptance criteria:
- Ví dụ dùng trong README.
- Unit tests: mapping level, convert Attrs (kể cả group tối thiểu), race-free.

Kiểm thử tối thiểu:
- slog.Info/slog.Warn route đúng stdout/stderr (WARN→stderr).
- Attr number/bool/string/time xuất đúng trong JSON.

Gợi ý:
- Handler giữ `LoggerWithCtx`; implement Enabled, Handle, WithAttrs, WithGroup.
- Dùng API hiện có để push vào pipeline, merge Attrs vào context.
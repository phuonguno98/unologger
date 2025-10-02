# Prompt: API ghi log dạng structured

Mục tiêu:
- Cung cấp API ghi log structured: `InfoFields/WarnFields/ErrorFields/FatalFields` nhận `msg string` và `Fields`.

Phạm vi:
- Thêm các phương thức trên `Logger` và `LoggerWithCtx`.
- JSONFormatter giữ nguyên `Fields` là object, `message` tách riêng; không stringify toàn bộ payload.

Ràng buộc:
- Backward-compatible với API cũ; masking field-level vẫn hoạt động.

Acceptance criteria:
- Unit tests: JSON output có fields theo đúng kiểu; message là field riêng; masking field-level xảy ra như mong đợi.

Kiểm thử tối thiểu:
- Gọi `InfoFields` với vài typed fields (int/bool/time) và xác minh JSON.
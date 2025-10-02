# Prompt: API Health snapshot cho unologger

Mục tiêu:
- Cung cấp `Health()` trả về snapshot: queueLen, counters, writerErrs, hookErrLog.

Phạm vi:
- Thêm `type Health struct { ... }` và `Health()/HealthDetached(*Logger)`.
- Đảm bảo thread-safe, không chặn pipeline.

Ràng buộc:
- Không panic khi logger đã đóng.

Acceptance criteria:
- Unit tests: giá trị hợp lệ trong lúc hoạt động và sau khi đóng.
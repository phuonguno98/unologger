# Prompt: Tuỳ chọn buffer writers và flush định kỳ

Mục tiêu:
- Giảm syscall bằng cách bọc writers vào `bufio.Writer` khi bật cấu hình.

Phạm vi:
- Thêm `BufferWriters bool`, `FlushInterval time.Duration` vào `Config`.
- Khi bật, bọc stdout/stderr/rotation/extras bằng bufio; flush theo interval và khi batch flush/close.

Ràng buộc:
- Đảm bảo Close luôn flush hoàn tất.

Acceptance criteria:
- Unit tests: flush sau interval và flush khi Close; không mất log khi app exit.